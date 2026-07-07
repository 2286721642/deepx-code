package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
)

// TestApplyTheme 验证背景明暗切换:首次落地、幂等守卫、亮/暗色值互换。
func TestApplyTheme(t *testing.T) {
	// 保存并在结束时恢复全局主题态,避免污染同包其它测试。
	savedDark, savedApplied := darkBackground, themeApplied
	t.Cleanup(func() {
		darkBackground, themeApplied = savedDark, savedApplied
		applyThemeForce()
	})

	// 从一个已知的"暗色且已初始化"起点开始。
	darkBackground, themeApplied = true, false
	if !applyTheme(true) {
		t.Fatal("首次 applyTheme 应返回 true(即便档位=默认暗,也要落地一次 rebuildStyles)")
	}
	if got := userBubbleFg; got != lipgloss.Color("231") {
		t.Fatalf("暗色档 userBubbleFg = %v,want 231", got)
	}
	if !darkBackground {
		t.Fatal("applyTheme(true) 后 darkBackground 应为 true")
	}

	// 同档再调 → 幂等,不重复切换。
	if applyTheme(true) {
		t.Fatal("相同档位重复 applyTheme 应返回 false")
	}

	// 切到亮色:近白前景应压深,darkBackground 翻转。
	if !applyTheme(false) {
		t.Fatal("切到亮色 applyTheme(false) 应返回 true")
	}
	if darkBackground {
		t.Fatal("applyTheme(false) 后 darkBackground 应为 false")
	}
	if got := userBubbleFg; got != lipgloss.Color("236") {
		t.Fatalf("亮色档 userBubbleFg = %v,want 236(深灰,白底可读)", got)
	}
	if got := highlightColor; got != lipgloss.Color("31") {
		t.Fatalf("亮色档 highlightColor = %v,want 31(深青)", got)
	}
	if got := softFgColor; got != lipgloss.Color("238") {
		t.Fatalf("亮色档 softFgColor = %v,want 238", got)
	}
}

// applyThemeForce 强制按当前 darkBackground 重新落地一次(测试清理用:绕过幂等守卫)。
func applyThemeForce() {
	themeApplied = false
	applyTheme(darkBackground)
}
