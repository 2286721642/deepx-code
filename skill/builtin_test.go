package skill

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 内置 superpowers 工作流 skill 必须能被解压、发现,且 frontmatter 完整。
func TestBuiltinSuperpowersSkills(t *testing.T) {
	home := t.TempDir()
	dest, err := ExtractBuiltins(home)
	if err != nil {
		t.Fatalf("ExtractBuiltins: %v", err)
	}

	loader := New(nil, []string{dest})
	got := map[string]Metadata{}
	for _, m := range loader.List() {
		got[m.Name] = m
	}

	// 保留的行为类子集(其余 superpowers skill 已按需裁掉)。
	want := []string{
		"brainstorming",
		"verification-before-completion",
	}
	for _, name := range want {
		m, ok := got[name]
		if !ok {
			t.Errorf("内置 skill %q 未被发现", name)
			continue
		}
		if strings.TrimSpace(m.Description) == "" {
			t.Errorf("skill %q 的 description 为空", name)
		}
		// 正文应能完整加载且非空
		s, err := loader.Load(name)
		if err != nil {
			t.Errorf("加载 skill %q: %v", name, err)
			continue
		}
		if strings.TrimSpace(s.Body) == "" {
			t.Errorf("skill %q 正文为空", name)
		}
	}
}

// 清理逻辑必须:删掉"上次是内置、这次没了"的废弃内置,且不碰用户自定义 skill。
func TestPruneRemovedBuiltins(t *testing.T) {
	dest := t.TempDir()

	// 模拟上一版解压结果:两个内置 + 清单,外加一个用户自定义 skill。
	mustMkSkill := func(name string) {
		if err := os.MkdirAll(filepath.Join(dest, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dest, name, "SKILL.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustMkSkill("old-builtin")  // 内置,本次将被移除
	mustMkSkill("kept-builtin") // 内置,本次仍在
	mustMkSkill("user-custom")  // 用户自定义,绝不能动
	writeBuiltinManifest(dest, []string{"old-builtin", "kept-builtin"})

	// 本次 embed 只剩 kept-builtin。
	pruneRemovedBuiltins(dest, []string{"kept-builtin"})

	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dest, name))
		return err == nil
	}
	if exists("old-builtin") {
		t.Error("废弃内置 old-builtin 应被删除,但仍存在")
	}
	if !exists("kept-builtin") {
		t.Error("保留的内置 kept-builtin 被误删")
	}
	if !exists("user-custom") {
		t.Error("用户自定义 user-custom 被误删 —— 清理逻辑越界了")
	}
}
