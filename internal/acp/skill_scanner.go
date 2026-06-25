package acp

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Skill 表示一个已发现的 Agent Skill（agentskills.io 规范）。
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Location    string `json:"location"` // SKILL.md 的绝对路径
	Scope       string `json:"scope"`    // "project" | "user"
}

// skillFrontmatter 是 SKILL.md 文件头部的 YAML frontmatter。
type skillFrontmatter struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

// skillScanDirs 返回需要扫描的 skill 目录列表（按优先级排序，project 优先于 user）。
// cwd 为会话工作目录，home 为用户主目录。
func skillScanDirs(cwd, home string) []struct {
	Path  string
	Scope string
} {
	var dirs []struct {
		Path  string
		Scope string
	}
	if cwd != "" {
		dirs = append(dirs,
			struct {
				Path  string
				Scope string
			}{filepath.Join(cwd, ".agents", "skills"), "project"},
			struct {
				Path  string
				Scope string
			}{filepath.Join(cwd, ".claude", "skills"), "project"},
		)
	}
	if home != "" {
		dirs = append(dirs,
			struct {
				Path  string
				Scope string
			}{filepath.Join(home, ".agents", "skills"), "user"},
			struct {
				Path  string
				Scope string
			}{filepath.Join(home, ".claude", "skills"), "user"},
		)
	}
	return dirs
}

// ScanSkills 扫描指定工作目录和用户主目录下的 Agent Skills。
// 发现的 skill 按 name 去重（project 级别优先于 user 级别）。
func ScanSkills(cwd string) []Skill {
	home, _ := os.UserHomeDir()
	scanDirs := skillScanDirs(cwd, home)

	seen := make(map[string]bool)
	var skills []Skill

	for _, dir := range scanDirs {
		entries, err := os.ReadDir(dir.Path)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			name := entry.Name()
			// 跳过隐藏目录
			if strings.HasPrefix(name, ".") {
				continue
			}
			// project 级别优先，已发现则跳过 user 级别同名 skill
			if seen[name] {
				continue
			}

			skillPath := filepath.Join(dir.Path, name, "SKILL.md")
			content, err := os.ReadFile(skillPath)
			if err != nil {
				continue
			}

			parsed, ok := parseSkillMarkdown(content)
			if !ok {
				continue
			}

			// name 以 frontmatter 为准，若为空则用目录名
			skillName := parsed.Name
			if skillName == "" {
				skillName = name
			}

			seen[skillName] = true
			skills = append(skills, Skill{
				Name:        skillName,
				Description: parsed.Description,
				Location:    skillPath,
				Scope:       dir.Scope,
			})
		}
	}

	return skills
}

// parseSkillMarkdown 解析 SKILL.md 文件，提取 YAML frontmatter。
// 返回 frontmatter 和是否成功解析。
func parseSkillMarkdown(content []byte) (skillFrontmatter, bool) {
	text := string(content)

	// 检查是否以 --- 开头
	if !strings.HasPrefix(text, "---") {
		return skillFrontmatter{}, false
	}

	// 找到第二个 ---
	rest := text[3:]
	// 跳过 --- 后的换行
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return skillFrontmatter{}, false
	}

	frontmatterText := rest[:endIdx]

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(frontmatterText), &fm); err != nil {
		return skillFrontmatter{}, false
	}

	// description 为空的 skill 无法用于 progressive disclosure，跳过
	if fm.Description == "" {
		return skillFrontmatter{}, false
	}

	return fm, true
}
