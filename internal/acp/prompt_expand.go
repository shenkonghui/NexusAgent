package acp

import (
	"os"
	"strings"

	"github.com/coder/acp-go-sdk"
)

const (
	expandPreamble = "Follow these instructions:\n\n"
	expandUserReq  = "\n\nUser request:\n"
)

// ExpandPromptInput 是 slash 调用展开所需的上下文。
type ExpandPromptInput struct {
	Prompt             string
	Cwd                string
	SkillUserDirs      []string
	SkillProjectDirs   []string
	CommandUserDirs    []string
	CommandProjectDirs []string
	AgentCommands      []acp.AvailableCommand
	Modes              []acp.SessionMode
}

// ExpandPrompt 若 prompt 以 /name 开头且匹配配置的 command 或 skill，则展开文件内容。
// 返回 (original, expanded)；无需展开时 expanded 与 original 相同。
func ExpandPrompt(in ExpandPromptInput) (original, expanded string) {
	original = in.Prompt
	expanded = in.Prompt

	name, rest, ok := parseSlashInvoke(in.Prompt)
	if !ok || isAgentReserved(name, in.AgentCommands, in.Modes) {
		return original, expanded
	}

	if loc := lookupSlashCommandLocation(name, in.Cwd, in.CommandUserDirs, in.CommandProjectDirs); loc != "" {
		if body, err := os.ReadFile(loc); err == nil {
			expanded = formatExpandedPrompt(string(body), rest)
			return original, expanded
		}
	}
	if loc := lookupSkillLocation(name, in.Cwd, in.SkillUserDirs, in.SkillProjectDirs); loc != "" {
		if body, err := os.ReadFile(loc); err == nil {
			expanded = formatExpandedPrompt(string(body), rest)
			return original, expanded
		}
	}
	return original, expanded
}

func parseSlashInvoke(prompt string) (name, rest string, ok bool) {
	prompt = strings.TrimSpace(prompt)
	if !strings.HasPrefix(prompt, "/") {
		return "", "", false
	}
	end := 1
	for end < len(prompt) && !isSlashNameBreak(prompt[end]) {
		end++
	}
	name = prompt[1:end]
	if name == "" {
		return "", "", false
	}
	return name, strings.TrimSpace(prompt[end:]), true
}

func isSlashNameBreak(b byte) bool {
	return b == ' ' || b == '\n' || b == '\t' || b == '\r'
}

func isAgentReserved(name string, cmds []acp.AvailableCommand, modes []acp.SessionMode) bool {
	for _, c := range cmds {
		if strings.EqualFold(c.Name, name) {
			return true
		}
	}
	for _, m := range modes {
		if strings.EqualFold(string(m.Id), name) || strings.EqualFold(m.Name, name) {
			return true
		}
	}
	return false
}

func lookupSlashCommandLocation(name, cwd string, userDirs, projectDirs []string) string {
	for _, c := range ScanSlashCommands(cwd, userDirs, projectDirs) {
		if strings.EqualFold(c.Name, name) {
			return c.Location
		}
	}
	return ""
}

func lookupSkillLocation(name, cwd string, userDirs, projectDirs []string) string {
	for _, sk := range ScanSkills(cwd, userDirs, projectDirs) {
		if strings.EqualFold(sk.Name, name) {
			return sk.Location
		}
	}
	return ""
}

func formatExpandedPrompt(instructions, userRest string) string {
	instructions = strings.TrimSpace(instructions)
	if userRest == "" {
		return expandPreamble + instructions
	}
	return expandPreamble + instructions + expandUserReq + userRest
}
