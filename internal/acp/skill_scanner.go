package acp

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"nexusagent/internal/config"
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

// ScanSkills 扫描配置目录下的 Agent Skills（支持子目录）。
// 发现的 skill 按 name 去重（project 级别优先于 user 级别）。
func ScanSkills(cwd string, cfg config.SkillsConfig) []Skill {
	home, _ := os.UserHomeDir()
	scanDirs := skillScanEntries(cwd, home, cfg)

	seen := make(map[string]bool)
	var skills []Skill

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
			if !strings.EqualFold(d.Name(), "SKILL.md") {
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			parsed, ok := parseSkillMarkdown(content)
			if !ok {
				return nil
			}

			skillName := parsed.Name
			if skillName == "" {
				relDir, err := filepath.Rel(dir.Path, filepath.Dir(path))
				if err != nil || relDir == "." {
					skillName = filepath.Base(filepath.Dir(path))
				} else {
					skillName = filepath.ToSlash(relDir)
				}
			}
			if seen[skillName] {
				return nil
			}

			seen[skillName] = true
			skills = append(skills, Skill{
				Name:        skillName,
				Description: parsed.Description,
				Location:    path,
				Scope:       dir.Scope,
			})
			return nil
		})
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
