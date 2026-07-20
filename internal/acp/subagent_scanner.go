package acp

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SubAgentDef 表示一个已发现的 subagent 定义（来自 markdown 文件，对齐 ~/.claude/agents 规范）。
//
// 一个 subagent 文件由 YAML frontmatter（name/description/model/tools）+ markdown 正文构成，
// 正文整体作为注入会话的 system_prompt。
type SubAgentDef struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Model        string   `json:"model,omitempty"`
	Tools        []string `json:"tools,omitempty"`
	SystemPrompt string   `json:"-"` // 正文，不对外暴露给 list/get 的 JSON
	Location     string   `json:"location"` // .md 文件绝对路径
	Scope        string   `json:"scope"`    // "project" | "user"
	Path         string   `json:"path"`     // 相对扫描根目录的展示路径
}

// subAgentFrontmatter 是 subagent .md 文件头部的 YAML frontmatter。
type subAgentFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Model       string `yaml:"model"`
	// RawTools 用 yaml.Node 承载，兼容两种写法：
	//   - 逗号分隔字符串（对齐 bug-analyzer.md：tools: read_file, write_file）
	//   - YAML 列表（对齐 frontmatter 常规写法：tools:\n  - read_file）
	// 留给 parseSubAgentMarkdown 统一规整为 []string。
	RawTools yaml.Node `yaml:"tools"`
}

// asToolsList 把 frontmatter 中的 tools 节点解析为 []string，兼容 scalar（逗号串）与 sequence。
func asToolsList(node *yaml.Node) []string {
	if node == nil || node.Kind == 0 {
		return nil
	}
	switch node.Kind {
	case yaml.ScalarNode:
		// 逗号分隔字符串
		var s string
		if err := node.Decode(&s); err != nil {
			return nil
		}
		return splitAndTrim(s)
	case yaml.SequenceNode:
		var items []string
		if err := node.Decode(&items); err != nil {
			return nil
		}
		return normalizeTools(items)
	default:
		return nil
	}
}

// splitAndTrim 按逗号拆分并 trim。
func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ScanSubAgents 扫描工作区与用户配置的 subagents 目录（递归子目录中的 *.md）。
// userDirs 为绝对路径；projectSubdirs 为相对 cwd 的子目录。project 优先于 user，按 name 去重。
func ScanSubAgents(cwd string, userDirs, projectSubdirs []string) []SubAgentDef {
	scanDirs := skillScanDirs(cwd, userDirs, projectSubdirs)

	seen := make(map[string]bool)
	var defs []SubAgentDef

	for _, dir := range scanDirs {
		scanSubAgentsUnder(dir.Path, dir.Path, dir.Scope, seen, &defs)
	}
	return defs
}

// scanSubAgentsUnder 递归扫描目录树，发现 *.md 文件即尝试解析为 subagent。
func scanSubAgentsUnder(root, scanRoot, scope string, seen map[string]bool, defs *[]SubAgentDef) {
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
			scanSubAgentsUnder(entryPath, scanRoot, scope, seen, defs)
			continue
		}
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		content, err := os.ReadFile(entryPath)
		if err != nil {
			continue
		}
		def, ok := parseSubAgentMarkdown(name, content)
		if !ok {
			continue
		}
		if seen[def.Name] {
			continue
		}
		seen[def.Name] = true
		def.Location = entryPath
		def.Scope = scope
		def.Path = commandDisplayPath(entryPath, scanRoot)
		*defs = append(*defs, def)
	}
}

// parseSubAgentMarkdown 解析 subagent .md 文件。
// 要求 frontmatter 中 description 非空；正文（strip frontmatter 后）作为 SystemPrompt。
func parseSubAgentMarkdown(filename string, content []byte) (SubAgentDef, bool) {
	base := strings.TrimSuffix(filename, filepath.Ext(filename))
	text := string(content)

	def := SubAgentDef{}
	hasFrontmatter := false

	if strings.HasPrefix(text, "---") {
		rest := text[3:]
		if strings.HasPrefix(rest, "\r\n") {
			rest = rest[2:]
		} else if strings.HasPrefix(rest, "\n") {
			rest = rest[1:]
		}
		if endIdx := strings.Index(rest, "\n---"); endIdx >= 0 {
			var fm subAgentFrontmatter
			if yaml.Unmarshal([]byte(rest[:endIdx]), &fm) == nil {
				hasFrontmatter = true
				def.Name = strings.TrimSpace(fm.Name)
				def.Description = strings.TrimSpace(fm.Description)
				def.Model = strings.TrimSpace(fm.Model)
				def.Tools = asToolsList(&fm.RawTools)
			}
		}
	}

	if def.Name == "" {
		def.Name = base
	}
	// description 必填（给主 agent 的调用依据）
	if strings.TrimSpace(def.Description) == "" {
		return SubAgentDef{}, false
	}

	body := stripSubAgentFrontmatter(content)
	def.SystemPrompt = strings.TrimSpace(body)
	_ = hasFrontmatter
	return def, true
}

// normalizeTools 清洗 tools 列表：trim、丢弃空串。
// 调用方传入的 fm.Tools 可能是 YAML 列表（已是 []string），也可能因用户写逗号字符串而被 yaml 解析为单元素。
// 这里额外兼容"单元素中含逗号"的写法（对齐 bug-analyzer.md：tools: read_file, write_file）。
func normalizeTools(raw []string) []string {
	if len(raw) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	for _, item := range raw {
		// 兼容 "a, b, c" 写法（yaml 把未加引号的逗号串当成一个标量）
		if strings.Contains(item, ",") {
			for _, part := range strings.Split(item, ",") {
				add(part)
			}
			continue
		}
		add(item)
	}
	return out
}

// stripSubAgentFrontmatter 去掉 markdown 头部的 YAML frontmatter，返回正文。
func stripSubAgentFrontmatter(content []byte) string {
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
	rest = rest[endIdx+4:]
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	return rest
}
