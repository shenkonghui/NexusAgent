package acp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/coder/acp-go-sdk"
	"gopkg.in/yaml.v3"
)

// SlashCommand 表示一个已发现的 Slash Command（Claude Code 规范：commands 目录下的 .md 文件）。
type SlashCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"` // .md 文件的绝对路径
	Scope       string `json:"scope"`    // "project" | "user"
	Path        string `json:"path"`     // 相对扫描根目录的展示路径，如 nested/deploy
}

type commandFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
}

// ScanSlashCommands 扫描工作区与用户配置的 commands 目录（Claude Code：递归 *.md，支持子目录与符号链接目录）。
// 命令名取自文件名（不含 .md）；子目录仅用于组织，不改变命令名。
func ScanSlashCommands(cwd string, userDirs, projectSubdirs []string) []SlashCommand {
	scanDirs := skillScanDirs(cwd, userDirs, projectSubdirs)

	seen := make(map[string]bool)
	var commands []SlashCommand

	for _, dir := range scanDirs {
		scanCommandsUnder(dir.Path, dir.Path, dir.Scope, seen, &commands)
	}
	return commands
}

func scanCommandsUnder(root, scanRoot, scope string, seen map[string]bool, commands *[]SlashCommand) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		entryPath := filepath.Join(root, name)
		if isScanEntryDir(entryPath, entry) {
			scanCommandsUnder(entryPath, scanRoot, scope, seen, commands)
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		content, err := os.ReadFile(entryPath)
		if err != nil {
			continue
		}
		cmdName, desc := parseCommandMarkdown(name, content)
		if cmdName == "" {
			continue
		}
		if seen[cmdName] {
			continue
		}
		seen[cmdName] = true
		*commands = append(*commands, SlashCommand{
			Name:        cmdName,
			Description: desc,
			Location:    entryPath,
			Scope:       scope,
			Path:        commandDisplayPath(entryPath, scanRoot),
		})
	}
}

func parseCommandMarkdown(filename string, content []byte) (name, description string) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	text := string(content)

	if strings.HasPrefix(text, "---") {
		rest := text[3:]
		if strings.HasPrefix(rest, "\r\n") {
			rest = rest[2:]
		} else if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		}
		if endIdx := strings.Index(rest, "\n---"); endIdx >= 0 {
			var fm commandFrontmatter
			if yaml.Unmarshal([]byte(rest[:endIdx]), &fm) == nil {
				name = strings.TrimSpace(fm.Name)
				description = strings.TrimSpace(fm.Description)
				text = strings.TrimSpace(rest[endIdx+4:])
				if strings.HasPrefix(text, "\r\n") {
					text = strings.TrimSpace(text[2:])
				} else if strings.HasPrefix(text, "\n") {
					text = strings.TrimSpace(text[1:])
				}
			}
		}
	}
	if name == "" {
		name = base
	}
	if description == "" {
		description = firstNonEmptyLine(text)
	}
	return name, description
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			if len(line) > 120 {
				return line[:120] + "…"
			}
			return line
		}
	}
	return ""
}

// SlashCommandsToAvailable 将扫描结果转为 ACP AvailableCommand（供 UI / 补全使用）。
func SlashCommandsToAvailable(commands []SlashCommand) []acp.AvailableCommand {
	out := make([]acp.AvailableCommand, 0, len(commands))
	for _, c := range commands {
		out = append(out, acp.AvailableCommand{
			Name:        c.Name,
			Description: c.Description,
		})
	}
	return out
}

// MergeAvailableCommands 合并 Agent 原生命令与配置的 slash command；同名时 Agent 优先。
func MergeAvailableCommands(agent, configured []acp.AvailableCommand) []acp.AvailableCommand {
	seen := make(map[string]bool, len(agent))
	out := make([]acp.AvailableCommand, 0, len(agent)+len(configured))
	for _, c := range agent {
		seen[c.Name] = true
		out = append(out, c)
	}
	for _, c := range configured {
		if seen[c.Name] {
			continue
		}
		out = append(out, c)
	}
	return out
}
