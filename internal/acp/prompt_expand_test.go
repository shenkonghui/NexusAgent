package acp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"
)

func TestExpandPrompt_SlashCommand(t *testing.T) {
	root := t.TempDir()
	cmdDir := filepath.Join(root, "cmds")
	if err := os.MkdirAll(cmdDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmdFile := filepath.Join(cmdDir, "deploy.md")
	if err := os.WriteFile(cmdFile, []byte("Deploy steps here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, expanded := ExpandPrompt(ExpandPromptInput{
		Prompt:          "/deploy to prod",
		CommandUserDirs: []string{cmdDir},
	})
	if !strings.Contains(expanded, "Deploy steps here") || !strings.Contains(expanded, "to prod") {
		t.Fatalf("展开异常: %q", expanded)
	}
}

func TestExpandPrompt_Skill(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "brainstorm")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: brainstorming\ndescription: ideate\n---\nBrainstorm first\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, expanded := ExpandPrompt(ExpandPromptInput{
		Prompt:        "/brainstorming design UI",
		SkillUserDirs: []string{root},
	})
	if !strings.Contains(expanded, "Brainstorm first") || !strings.Contains(expanded, "design UI") {
		t.Fatalf("展开异常: %q", expanded)
	}
}

func TestExpandPrompt_AgentCommandReserved(t *testing.T) {
	_, expanded := ExpandPrompt(ExpandPromptInput{
		Prompt: "/commit fix bug",
		AgentCommands: []acpsdk.AvailableCommand{
			{Name: "commit", Description: "native"},
		},
	})
	if expanded != "/commit fix bug" {
		t.Fatalf("Agent 原生命令不应展开: %q", expanded)
	}
}

func TestExpandPrompt_PlainText(t *testing.T) {
	_, expanded := ExpandPrompt(ExpandPromptInput{Prompt: "hello world"})
	if expanded != "hello world" {
		t.Fatalf("普通文本不应改变: %q", expanded)
	}
}
