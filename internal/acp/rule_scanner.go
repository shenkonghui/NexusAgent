package acp

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Rule 表示一个已发现的 Cursor Rule（.cursor/rules 下的 .mdc / .md）。
type Rule struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Scope       string `json:"scope"` // "project" | "user"
	AlwaysApply bool   `json:"always_apply"`
	Globs       string `json:"globs,omitempty"`
}

type ruleFrontmatter struct {
	Description string `yaml:"description"`
	AlwaysApply bool   `yaml:"alwaysApply"`
	Globs       string `yaml:"globs"`
}

// ScanRules 扫描配置的 rules 路径（目录递归 *.mdc / *.md，或直接指定文件如 CLAUDE.md）。
func ScanRules(cwd string, userPaths, projectPaths []string) []Rule {
	seen := make(map[string]bool)
	var rules []Rule

	for _, entry := range ruleScanEntries(cwd, userPaths, projectPaths) {
		if entry.IsFile {
			appendRuleFromFile(entry.Path, entry.Scope, true, seen, &rules)
			continue
		}
		scanRulesUnder(entry.Path, entry.Scope, seen, &rules)
	}
	return rules
}

type ruleScanEntry struct {
	Path   string
	Scope  string
	IsFile bool
}

// ruleScanEntries 解析配置路径；存在且为文件则 IsFile=true，为目录则递归扫描。
func ruleScanEntries(cwd string, userPaths, projectPaths []string) []ruleScanEntry {
	var entries []ruleScanEntry
	if cwd != "" {
		for _, sub := range projectPaths {
			sub = strings.TrimSpace(sub)
			if sub == "" {
				continue
			}
			if e, ok := resolveRuleScanEntry(filepath.Join(cwd, filepath.FromSlash(sub)), "project"); ok {
				entries = append(entries, e)
			}
		}
	}
	for _, p := range userPaths {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if e, ok := resolveRuleScanEntry(p, "user"); ok {
			entries = append(entries, e)
		}
	}
	return entries
}

func resolveRuleScanEntry(path, scope string) (ruleScanEntry, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return ruleScanEntry{}, false
	}
	return ruleScanEntry{Path: path, Scope: scope, IsFile: !info.IsDir()}, true
}

// RuleAdditionalDirectories 返回 rules 应对 Agent 暴露的目录（文件取其父目录）。
func RuleAdditionalDirectories(cwd string, userPaths, projectPaths []string) []string {
	seen := make(map[string]bool)
	var dirs []string
	cwdAbs, _ := filepath.Abs(cwd)

	addDir := func(path string) {
		path = filepath.Clean(path)
		if path == "" {
			return
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return
		}
		if cwdAbs != "" && abs == cwdAbs {
			return
		}
		if seen[abs] {
			return
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return
		}
		seen[abs] = true
		dirs = append(dirs, abs)
	}

	for _, entry := range ruleScanEntries(cwd, userPaths, projectPaths) {
		if entry.IsFile {
			addDir(filepath.Dir(entry.Path))
		} else {
			addDir(entry.Path)
		}
	}
	return dirs
}

func appendRuleFromFile(path, scope string, defaultAlwaysApply bool, seen map[string]bool, rules *[]Rule) {
	if !isRuleFile(path) {
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	if seen[abs] {
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return
	}
	ruleName, desc, alwaysApply, globs := parseRuleMarkdown(filepath.Base(path), content, defaultAlwaysApply)
	if ruleName == "" {
		return
	}
	seen[abs] = true
	*rules = append(*rules, Rule{
		Name:        ruleName,
		Description: desc,
		Location:    abs,
		Scope:       scope,
		AlwaysApply: alwaysApply,
		Globs:       globs,
	})
}

func isRuleFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".mdc" || ext == ".md"
}

func scanRulesUnder(root, scope string, seen map[string]bool, rules *[]Rule) {
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
		if entry.IsDir() {
			scanRulesUnder(entryPath, scope, seen, rules)
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".mdc" && ext != ".md" {
			continue
		}
		content, err := os.ReadFile(entryPath)
		if err != nil {
			continue
		}
		abs, err := filepath.Abs(entryPath)
		if err != nil || seen[abs] {
			continue
		}
		ruleName, desc, alwaysApply, globs := parseRuleMarkdown(name, content, false)
		if ruleName == "" {
			continue
		}
		seen[abs] = true
		*rules = append(*rules, Rule{
			Name:        ruleName,
			Description: desc,
			Location:    abs,
			Scope:       scope,
			AlwaysApply: alwaysApply,
			Globs:       globs,
		})
	}
}

func parseRuleMarkdown(filename string, content []byte, defaultAlwaysApply bool) (name, description string, alwaysApply bool, globs string) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	text := string(content)
	hasFrontmatter := false

	if strings.HasPrefix(text, "---") {
		rest := text[3:]
		if strings.HasPrefix(rest, "\r\n") {
			rest = rest[2:]
		} else if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		}
		if endIdx := strings.Index(rest, "\n---"); endIdx >= 0 {
			var fm ruleFrontmatter
			if yaml.Unmarshal([]byte(rest[:endIdx]), &fm) == nil {
				hasFrontmatter = true
				description = strings.TrimSpace(fm.Description)
				alwaysApply = fm.AlwaysApply
				globs = strings.TrimSpace(fm.Globs)
			}
		}
	}
	if name == "" {
		name = base
	}
	if !hasFrontmatter {
		alwaysApply = defaultAlwaysApply
	}
	return name, description, alwaysApply, globs
}

// AlwaysApplySystemPrompt 汇总 alwaysApply 规则正文，供 session/new 的 _meta.systemPrompt 注入。
// 无匹配规则时返回空字符串。
func AlwaysApplySystemPrompt(cwd string, userDirs, projectDirs []string) string {
	var bodies []string
	for _, r := range ScanRules(cwd, userDirs, projectDirs) {
		if !r.AlwaysApply {
			continue
		}
		content, err := os.ReadFile(r.Location)
		if err != nil {
			continue
		}
		if body := strings.TrimSpace(stripRuleFrontmatter(content)); body != "" {
			bodies = append(bodies, body)
		}
	}
	if len(bodies) == 0 {
		return ""
	}
	return strings.Join(bodies, "\n\n---\n\n")
}

func stripRuleFrontmatter(content []byte) string {
	text := string(content)
	if !strings.HasPrefix(text, "---") {
		return text
	}
	rest := text[3:]
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return text
	}
	rest = strings.TrimSpace(rest[endIdx+4:])
	if strings.HasPrefix(rest, "\r\n") {
		rest = strings.TrimSpace(rest[2:])
	} else if strings.HasPrefix(rest, "\n") {
		rest = strings.TrimSpace(rest[1:])
	}
	return rest
}
