package acp

import (
	"os"
	"path/filepath"
	"testing"

	"nexusagent/internal/config"
)

func writeSkill(t *testing.T, dir, name, content string) {
	t.Helper()
	skillDir := filepath.Join(dir, name)
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("创建 skill 目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("写入 SKILL.md: %v", err)
	}
}

func writeCommand(t *testing.T, dir, relPath, content string) {
	t.Helper()
	path := filepath.Join(dir, relPath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("创建 command 目录: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("写入 command: %v", err)
	}
}

func TestScanSkills_DefaultDirsAndNested(t *testing.T) {
	root := t.TempDir()
	writeSkill(t, filepath.Join(root, ".agents", "skills"), "top-skill", `---
name: top-skill
description: 顶层 skill
---
`)
	writeSkill(t, filepath.Join(root, ".agents", "skills", "nested"), "deep-skill", `---
name: deep-skill
description: 嵌套 skill
---
`)

	skills := ScanSkills(root, config.SkillsConfig{})
	if len(skills) != 2 {
		t.Fatalf("skills 数量 = %d, 期望 2", len(skills))
	}
	names := map[string]string{skills[0].Name: skills[0].Scope, skills[1].Name: skills[1].Scope}
	if names["top-skill"] != "project" || names["deep-skill"] != "project" {
		t.Errorf("scope 不正确: %+v", names)
	}
}

func TestScanSkills_CustomDirs(t *testing.T) {
	root := t.TempDir()
	custom := filepath.Join(root, "custom-skills")
	writeSkill(t, custom, "my-skill", `---
name: my-skill
description: 自定义目录
---
`)

	skills := ScanSkills(root, config.SkillsConfig{
		ProjectDirs: []string{"custom-skills"},
		UserDirs:    []string{},
	})
	if len(skills) != 1 || skills[0].Name != "my-skill" {
		t.Fatalf("skills = %+v", skills)
	}
}

func TestScanSkills_ProjectOverridesUser(t *testing.T) {
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeSkill(t, filepath.Join(root, ".claude", "skills"), "shared", `---
name: shared
description: project
---
`)
	writeSkill(t, filepath.Join(home, ".claude", "skills"), "shared", `---
name: shared
description: user
---
`)

	// 手动传入 home 路径测试优先级：ScanSkills 使用 os.UserHomeDir()
	// 这里通过 custom config 只扫描 project，单独验证 project 优先逻辑
	skills := ScanSkills(root, config.SkillsConfig{
		ProjectDirs: []string{".claude/skills"},
		UserDirs:    []string{".claude/skills"},
	})
	// 两个 scope 都有同名 skill，project 应先被扫描到
	if len(skills) != 1 {
		t.Fatalf("skills 数量 = %d, 期望 1", len(skills))
	}
	if skills[0].Scope != "project" || skills[0].Description != "project" {
		t.Errorf("project 未优先: %+v", skills[0])
	}
}

func TestScanSlashCommands_DefaultAndNested(t *testing.T) {
	root := t.TempDir()
	writeCommand(t, filepath.Join(root, ".cursor", "commands"), "review.md", "# 代码审查\n审查变更。")
	writeCommand(t, filepath.Join(root, ".cursor", "commands", "audit"), "security.md", "安全审计命令")

	cmds := ScanSlashCommands(root, config.SlashCommandsConfig{})
	if len(cmds) != 2 {
		t.Fatalf("commands 数量 = %d, 期望 2", len(cmds))
	}
	names := map[string]FileCommand{}
	for _, c := range cmds {
		names[c.Name] = c
	}
	if names["review"].Description != "代码审查" {
		t.Errorf("review description = %q", names["review"].Description)
	}
	if names["audit/security"].Name != "audit/security" {
		t.Errorf("嵌套 command 名称不正确: %+v", names["audit/security"])
	}
}

func TestScanSlashCommands_CustomDir(t *testing.T) {
	root := t.TempDir()
	writeCommand(t, filepath.Join(root, "my-cmds"), "deploy.md", "部署到生产环境")

	cmds := ScanSlashCommands(root, config.SlashCommandsConfig{
		ProjectDirs: []string{"my-cmds"},
	})
	if len(cmds) != 1 || cmds[0].Name != "deploy" {
		t.Fatalf("commands = %+v", cmds)
	}
}

func TestResolveScanPath(t *testing.T) {
	base := "/tmp/project"
	if got := resolveScanPath(base, "skills"); got != "/tmp/project/skills" {
		t.Errorf("相对路径 = %q", got)
	}
	if got := resolveScanPath(base, "/abs/skills"); got != "/abs/skills" {
		t.Errorf("绝对路径 = %q", got)
	}
	home, _ := os.UserHomeDir()
	if got := resolveScanPath(base, "~/.cursor/commands"); got != filepath.Join(home, ".cursor", "commands") {
		t.Errorf("~ 路径 = %q, 期望 %q", got, filepath.Join(home, ".cursor", "commands"))
	}
}
