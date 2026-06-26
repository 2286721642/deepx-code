package agent

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestClampToolOutput_SmallPassthrough(t *testing.T) {
	in := "ok: 3 files changed"
	if got := clampToolOutput("Bash", in); got != in {
		t.Fatalf("small output should pass through unchanged, got %q", got)
	}
}

func TestStripBase64Blobs_RemovesScreenshot(t *testing.T) {
	// 模拟 browser_screenshot:短前缀 + 一大段单行 base64。
	blob := strings.Repeat("A", 200000)
	in := "data:image/png;base64," + blob
	got := stripBase64Blobs(in)
	if strings.Contains(got, blob) {
		t.Fatalf("base64 blob should be stripped")
	}
	if !strings.Contains(got, "已省略") {
		t.Fatalf("expected placeholder, got %q", got)
	}
	if len(got) > 1024 {
		t.Fatalf("stripped output too large: %d bytes", len(got))
	}
}

func TestStripBase64Blobs_KeepsNormalText(t *testing.T) {
	// 正常代码 / 文本含空白和标点,不应被当作 base64 误删。
	in := strings.Repeat("func foo(x int) int { return x + 1 }\n", 500)
	if got := stripBase64Blobs(in); got != in {
		t.Fatalf("normal text must be preserved")
	}
}

func TestClampToolOutput_TruncatesHuge(t *testing.T) {
	// 巨大的纯文本(非 base64,因含空格不构成连续 base64 串)应被字节上限截断。
	in := strings.Repeat("word ", 500000) // ~2.5MB
	got := clampToolOutput("Read", in)
	if len(got) > maxToolOutputBytes+512 {
		t.Fatalf("output not clamped: %d bytes", len(got))
	}
	if !strings.Contains(got, "已截断") {
		t.Fatalf("expected truncation notice")
	}
	if !utf8.ValidString(got) {
		t.Fatalf("clamped output must be valid UTF-8")
	}
}

func TestClampToolOutput_UTF8Boundary(t *testing.T) {
	// 多字节字符正好跨越截断点时,不能截出半个 rune。
	in := strings.Repeat("中", maxToolOutputBytes) // 每个 3 字节,远超上限
	got := clampToolOutput("Read", in)
	if !utf8.ValidString(got) {
		t.Fatalf("clamped multibyte output must remain valid UTF-8")
	}
}

func TestClampTurnToolOutput_AggregateCap(t *testing.T) {
	// 模拟一轮里并发多条工具结果:单条都 ≤96KB(过了 clampToolOutput),
	// 但合计要被「本轮合计上限」收口。
	spent := 0
	chunk := strings.Repeat("x", maxToolOutputBytes) // 单条 96KB,符合单条上限

	round := 0
	for range 10 { // 10×96KB = 960KB,远超 256KB 合计上限
		out := clampTurnToolOutput("Read", chunk, &spent)
		round++
		if !utf8.ValidString(out) {
			t.Fatalf("第 %d 条结果应为合法 UTF-8", round)
		}
		if spent > maxToolOutputBytesPerTurn {
			t.Fatalf("spent 不应超过本轮合计上限:%d > %d", spent, maxToolOutputBytesPerTurn)
		}
	}
	// 合计真正写入历史的字节(spent)必须被压在上限内。
	if spent > maxToolOutputBytesPerTurn {
		t.Fatalf("本轮合计未收口:spent=%d", spent)
	}
	// 预算用尽后,后续结果应只剩简短占位(远小于一条 96KB)。
	last := clampTurnToolOutput("Read", chunk, &spent)
	if len(last) > 1024 {
		t.Fatalf("预算用尽后应只返回简短占位,got %d bytes", len(last))
	}
	if !strings.Contains(last, "未计入上下文") {
		t.Fatalf("预算用尽占位应提示未计入上下文,got %q", last)
	}
}

func TestClampTurnToolOutput_SmallPassthrough(t *testing.T) {
	// 预算充足时,单条小结果原样通过并正确累加 spent。
	spent := 0
	in := "ok: 3 files changed"
	if got := clampTurnToolOutput("Bash", in, &spent); got != in {
		t.Fatalf("预算充足的小结果应原样通过,got %q", got)
	}
	if spent != len(in) {
		t.Fatalf("spent 应累加为 %d,got %d", len(in), spent)
	}
}
