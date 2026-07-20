package acp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanSubAgents_ProjectPriority(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "user-agents")
	projectRoot := filepath.Join(root, "project")
	projectDir := filepath.Join(projectRoot, ".agents", "agents")

	if err := os.MkdirAll(userDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// user 与 project 同名：project 应胜出
	if err := os.WriteFile(filepath.Join(userDir, "shared.md"), []byte("---\nname: shared\ndescription: from user\n---\nUSER BODY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "shared.md"), []byte("---\nname: shared\ndescription: from project\n---\nPROJECT BODY\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 仅 user 有
	if err := os.WriteFile(filepath.Join(userDir, "only-user.md"), []byte("---\nname: only-user\ndescription: user only\n---\nU\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	defs := ScanSubAgents(projectRoot, []string{userDir}, []string{".agents/agents"})
	if len(defs) != 2 {
		t.Fatalf("期望 2 个 subagent，实际 %d: %+v", len(defs), defs)
	}
	byName := map[string]SubAgentDef{}
	for _, d := range defs {
		byName[d.Name] = d
	}
	shared, ok := byName["shared"]
	if !ok {
		t.Fatalf("未找到 shared，实际: %+v", defs)
	}
	if shared.Description != "from project" || shared.Scope != "project" {
		t.Fatalf("project 应优先，实际: %+v", shared)
	}
	if shared.SystemPrompt != "PROJECT BODY" {
		t.Fatalf("SystemPrompt 应取 project 文件正文，实际: %q", shared.SystemPrompt)
	}
	if _, ok := byName["only-user"]; !ok {
		t.Fatalf("应包含 only-user，实际: %+v", defs)
	}
}

func TestScanSubAgents_NestedSubdirs(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "agents")
	nested := filepath.Join(userDir, "group", "deep")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "nested.md"), []byte("---\nname: nested\ndescription: deep one\n---\nBODY\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	defs := ScanSubAgents("", []string{userDir}, nil)
	if len(defs) != 1 || defs[0].Name != "nested" {
		t.Fatalf("期望扫描嵌套 subagent，实际: %+v", defs)
	}
	if defs[0].Path != "group/deep/nested" {
		t.Fatalf("展示路径异常: %q", defs[0].Path)
	}
}

func TestScanSubAgents_SkipsNonMdAndDotDirs(t *testing.T) {
	root := t.TempDir()
	userDir := filepath.Join(root, "agents")
	if err := os.MkdirAll(filepath.Join(userDir, ".hidden"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, ".hidden", "secret.md"), []byte("---\nname: secret\ndescription: s\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "readme.txt"), []byte("not md"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userDir, "real.md"), []byte("---\nname: real\ndescription: ok\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	defs := ScanSubAgents("", []string{userDir}, nil)
	if len(defs) != 1 || defs[0].Name != "real" {
		t.Fatalf("应只扫到 real，实际: %+v", defs)
	}
}

func TestParseSubAgentMarkdown_FullFrontmatter(t *testing.T) {
	content := []byte("---\nname: bug-analyzer\ndescription: debug expert\nmodel: opus\ntools: read_file, write_file, run_bash\n---\n\n# Bug 分析\n\n你是调试专家。\n")
	def, ok := parseSubAgentMarkdown("ignored.md", content)
	if !ok {
		t.Fatalf("应解析成功")
	}
	if def.Name != "bug-analyzer" {
		t.Fatalf("Name 异常: %q", def.Name)
	}
	if def.Description != "debug expert" {
		t.Fatalf("Description 异常: %q", def.Description)
	}
	if def.Model != "opus" {
		t.Fatalf("Model 异常: %q", def.Model)
	}
	wantTools := []string{"read_file", "write_file", "run_bash"}
	if len(def.Tools) != len(wantTools) {
		t.Fatalf("Tools 异常: %+v", def.Tools)
	}
	for i, w := range wantTools {
		if def.Tools[i] != w {
			t.Fatalf("Tools[%d] 异常: got %q want %q", i, def.Tools[i], w)
		}
	}
	if def.SystemPrompt != "# Bug 分析\n\n你是调试专家。" {
		t.Fatalf("SystemPrompt 异常: %q", def.SystemPrompt)
	}
}

func TestParseSubAgentMarkdown_ToolsAsList(t *testing.T) {
	content := []byte("---\nname: a\ndescription: d\ntools:\n  - read\n  - write\n---\nBODY\n")
	def, ok := parseSubAgentMarkdown("a.md", content)
	if !ok {
		t.Fatalf("应解析成功")
	}
	if len(def.Tools) != 2 || def.Tools[0] != "read" || def.Tools[1] != "write" {
		t.Fatalf("YAML 列表写法的 tools 异常: %+v", def.Tools)
	}
}

func TestParseSubAgentMarkdown_NameFromFilename(t *testing.T) {
	content := []byte("---\ndescription: no name field\n---\nBODY\n")
	def, ok := parseSubAgentMarkdown("auto-name.md", content)
	if !ok {
		t.Fatalf("应解析成功")
	}
	if def.Name != "auto-name" {
		t.Fatalf("应从文件名推断 name，实际: %q", def.Name)
	}
}

func TestParseSubAgentMarkdown_DescriptionRequired(t *testing.T) {
	cases := [][]byte{
		[]byte("---\nname: a\n---\nBODY\n"),                      // 无 description
		[]byte("---\nname: a\ndescription: \"  \"\n---\nBODY\n"), // description 仅空白
		[]byte("# 无 frontmatter\nBODY\n"),                       // 无 frontmatter
	}
	for i, c := range cases {
		if _, ok := parseSubAgentMarkdown("a.md", c); ok {
			t.Fatalf("case %d 应解析失败（description 必填）", i)
		}
	}
}

func TestParseSubAgentMarkdown_NoFrontmatterBody(t *testing.T) {
	// 没 frontmatter 但有 description 必填校验 → 应失败
	content := []byte("纯正文，没有 frontmatter")
	if _, ok := parseSubAgentMarkdown("x.md", content); ok {
		t.Fatalf("无 frontmatter 应因 description 缺失而失败")
	}
}

func TestNormalizeTools_DedupAndTrim(t *testing.T) {
	got := normalizeTools([]string{"a", " b ", "a", "c,d", "", "  "})
	want := []string{"a", "b", "c", "d"}
	if len(got) != len(want) {
		t.Fatalf("期望 %+v，实际 %+v", want, got)
	}
	for i, w := range want {
		if got[i] != w {
			t.Fatalf("got[%d]=%q want %q", i, got[i], w)
		}
	}
}

func TestStripSubAgentFrontmatter_PreservesBody(t *testing.T) {
	content := []byte("---\nname: a\ndescription: d\n---\n\n第一行\n\n第二行\n")
	body := stripSubAgentFrontmatter(content)
	if body != "\n第一行\n\n第二行\n" {
		t.Fatalf("正文异常: %q", body)
	}
}

func TestStripSubAgentFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte("just body\n")
	if got := stripSubAgentFrontmatter(content); got != "just body\n" {
		t.Fatalf("无 frontmatter 应原样返回，实际: %q", got)
	}
}
