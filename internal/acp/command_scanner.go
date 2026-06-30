package acp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"nexusagent/internal/config"
)

// FileCommand 表示从文件系统扫描到的 slash command（Markdown 文件）。
type FileCommand struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Scope       string `json:"scope"`
}

// ScanSlashCommands 扫描配置目录下的 Markdown slash command 文件（支持子目录）。
// 文件名（不含 .md）作为命令名，子目录以 / 分隔（如 audit/review）。
func ScanSlashCommands(cwd string, cfg config.SlashCommandsConfig) []FileCommand {
	home, _ := os.UserHomeDir()
	scanDirs := commandScanEntries(cwd, home, cfg)

	seen := make(map[string]bool)
	var commands []FileCommand

	for _, dir := range scanDirs {
		info, err := os.Stat(dir.Path)
		if err != nil || !info.IsDir() {
			continue
		}
		_ = filepath.WalkDir(dir.Path, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				if path != dir.Path && isHiddenEntry(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.EqualFold(filepath.Ext(d.Name()), ".md") {
				return nil
			}
			if strings.EqualFold(d.Name(), "SKILL.md") {
				return nil
			}

			rel, err := filepath.Rel(dir.Path, path)
			if err != nil {
				return nil
			}
			name := strings.TrimSuffix(filepath.ToSlash(rel), ".md")
			if name == "" || seen[name] {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}

			seen[name] = true
			commands = append(commands, FileCommand{
				Name:        name,
				Description: firstMeaningfulLine(content),
				Location:    path,
				Scope:       dir.Scope,
			})
			return nil
		})
	}

	return commands
}

func firstMeaningfulLine(content []byte) string {
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line == "---" {
			continue
		}
		line = strings.TrimLeft(line, "#")
		line = strings.TrimSpace(line)
		if line != "" {
			return truncateRunes(line, 120)
		}
	}
	return ""
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
