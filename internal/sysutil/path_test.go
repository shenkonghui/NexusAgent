package sysutil

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func TestSplitPath(t *testing.T) {
	sep := string(filepath.ListSeparator)
	got := splitPath("a" + sep + " b " + sep + "" + sep + "c")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("splitPath = %v, want %v", got, want)
	}
}

func TestSplitPath_Empty(t *testing.T) {
	if got := splitPath("   "); got != nil {
		t.Errorf("splitPath(`   `) = %v, want nil", got)
	}
}

// TestDiscoverExtraPathEntries_Dedup 验证已存在于 PATH 的目录不会重复加入。
func TestDiscoverExtraPathEntries_Dedup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH 扩充仅在 unix 生效")
	}
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	// sub 已在 current 中 → 不应再次出现；loginShellPath/common 中已含的目录
	// 才会被加入。此处只校验去重语义：current 中的目录绝不重复返回。
	current := sub + string(filepath.ListSeparator) + "/nonexistent-dir"
	got := discoverExtraPathEntries(current)
	for _, d := range got {
		if d == sub {
			t.Errorf("已存在的目录 %q 不应再次出现", sub)
		}
	}
}

// TestDiscoverExtraPathEntries_AddsHomeLocal 验证 ~/.local/bin 在缺失时会被补齐。
func TestDiscoverExtraPathEntries_AddsHomeLocal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH 扩充仅在 unix 生效")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法获取 home 目录")
	}
	localBin := filepath.Join(home, ".local/bin")
	info, err := os.Stat(localBin)
	if err != nil || !info.IsDir() {
		t.Skip("~/.local/bin 不存在，跳过")
	}
	// current 故意不含 localBin
	current := "/usr/bin" + string(filepath.ListSeparator) + "/bin"
	if strings.Contains(current, localBin) {
		t.Skip("current 已含 localBin")
	}
	got := discoverExtraPathEntries(current)
	if !contains(got, localBin) {
		t.Errorf("期望补齐 %q, got %v", localBin, got)
	}
}

// TestDiscoverExtraPathEntries_OnlyExistingDirs 验证不存在的目录被过滤。
func TestDiscoverExtraPathEntries_OnlyExistingDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH 扩充仅在 unix 生效")
	}
	got := discoverExtraPathEntries("")
	for _, d := range got {
		info, err := os.Stat(d)
		if err != nil {
			t.Errorf("返回的目录 %q 不可访问: %v", d, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("返回的 %q 不是目录", d)
		}
	}
}

// TestEnrichPath_Idempotent 两次调用不应造成 PATH 重复或丢失原有条目。
func TestEnrichPath_Idempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PATH 扩充仅在 unix 生效")
	}
	original := os.Getenv("PATH")
	t.Cleanup(func() { _ = os.Setenv("PATH", original) })

	EnrichPath()
	first := os.Getenv("PATH")
	EnrichPath()
	second := os.Getenv("PATH")

	// 第二次不应再新增条目（都已合并）
	if countDuplicates(second) > countDuplicates(first) {
		t.Errorf("二次调用引入了重复条目: first=%q second=%q", first, second)
	}
	// 原始 PATH 中的所有条目应仍然存在
	for _, e := range splitPath(original) {
		if !strings.Contains(second, e) {
			t.Errorf("二次调用后丢失原始 PATH 条目 %q", e)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func countDuplicates(p string) int {
	parts := splitPath(p)
	seen := make(map[string]int, len(parts))
	for _, p := range parts {
		seen[p]++
	}
	dups := 0
	for _, n := range seen {
		if n > 1 {
			dups += n - 1
		}
	}
	return dups
}

// 编译期断言：sort 仅在测试代码中可用（防 goimports 误删）。
var _ = sort.Strings
