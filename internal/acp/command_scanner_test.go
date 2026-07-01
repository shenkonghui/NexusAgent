package acp

import (
	"os"
	"path/filepath"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestScanSlashCommands_ClaudeCodeLayout(t *testing.T) {
	root := t.TempDir()
	userCmds := filepath.Join(root, "user-commands")
	projectRoot := filepath.Join(root, "project")
	projectCmds := filepath.Join(projectRoot, ".claude", "commands")

	if err := os.MkdirAll(filepath.Join(userCmds, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(userCmds, "nested", "deploy.md"), []byte("Deploy to staging\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(projectCmds, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectCmds, "review.md"), []byte("---\ndescription: code review\n---\nReview changes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmds := ScanSlashCommands(projectRoot, []string{userCmds}, []string{".claude/commands"})
	if len(cmds) != 2 {
		t.Fatalf("期望 2 个 commands，实际 %d: %+v", len(cmds), cmds)
	}
	if cmds[0].Name != "review" || cmds[0].Scope != "project" {
		t.Fatalf("project command 异常: %+v", cmds[0])
	}
	if cmds[1].Name != "deploy" || cmds[1].Scope != "user" {
		t.Fatalf("user command 异常: %+v", cmds[1])
	}
}

func TestMergeAvailableCommands_AgentWins(t *testing.T) {
	agent := []acpsdk.AvailableCommand{{Name: "commit", Description: "agent"}}
	configured := []acpsdk.AvailableCommand{
		{Name: "commit", Description: "configured"},
		{Name: "deploy", Description: "configured deploy"},
	}
	merged := MergeAvailableCommands(agent, configured)
	if len(merged) != 2 {
		t.Fatalf("期望 2 项，实际 %d", len(merged))
	}
	if merged[0].Description != "agent" {
		t.Fatalf("同名应保留 agent 版本: %+v", merged[0])
	}
}

func TestParseCommandMarkdown_FrontmatterName(t *testing.T) {
	content := []byte("---\nname: custom-name\ndescription: from fm\n---\nBody")
	name, desc := parseCommandMarkdown("ignored.md", content)
	if name != "custom-name" || desc != "from fm" {
		t.Fatalf("解析异常: name=%q desc=%q", name, desc)
	}
}
