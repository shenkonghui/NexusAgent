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

type skillScanDir struct {
	Path  string
	Scope string
}

// skillFrontmatter 是 SKILL.md 文件头部的 YAML frontmatter。
type skillFrontmatter struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
}

// skillScanDirs 返回需要扫描的 skill 目录列表（project 优先于 user）。
func skillScanDirs(cwd string, userDirs, projectSubdirs []string) []skillScanDir {
	var dirs []skillScanDir
	if cwd != "" {
		for _, sub := range projectSubdirs {
			sub = strings.TrimSpace(sub)
			if sub == "" {
				continue
			}
			dirs = append(dirs, skillScanDir{
				Path:  filepath.Join(cwd, filepath.FromSlash(sub)),
				Scope: "project",
			})
		}
	}
	for _, dir := range userDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		dirs = append(dirs, skillScanDir{Path: dir, Scope: "user"})
	}
	return dirs
}

// MergeAdditionalDirectories 合并多组 additional 目录列表并去重。
func MergeAdditionalDirectories(lists ...[]string) []string {
	seen := make(map[string]bool)
	var dirs []string
	for _, list := range lists {
		for _, d := range list {
			if d == "" || seen[d] {
				continue
			}
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// SkillAdditionalDirectories 返回应作为 ACP additionalDirectories 传递的绝对路径。
// 包含存在的 user_dirs 与 cwd 下存在的 project_dirs；去重且不含 cwd 本身。
func SkillAdditionalDirectories(cwd string, userDirs, projectSubdirs []string) []string {
	seen := make(map[string]bool)
	var dirs []string
	cwdAbs, _ := filepath.Abs(cwd)

	add := func(path string) {
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

	for _, dir := range userDirs {
		add(dir)
	}
	if cwd != "" {
		for _, sub := range projectSubdirs {
			sub = strings.TrimSpace(sub)
			if sub == "" {
				continue
			}
			add(filepath.Join(cwd, filepath.FromSlash(sub)))
		}
	}
	return dirs
}

// ScanSkills 扫描工作区与用户配置的 skills 目录（递归子目录）。
// userDirs 为绝对路径；projectSubdirs 为相对 cwd 的子目录。
func ScanSkills(cwd string, userDirs, projectSubdirs []string) []Skill {
	scanDirs := skillScanDirs(cwd, userDirs, projectSubdirs)

	seen := make(map[string]bool)
	var skills []Skill

	for _, dir := range scanDirs {
		scanSkillsUnder(dir.Path, dir.Scope, seen, &skills)
	}

	return skills
}

// scanSkillsUnder 递归扫描目录树，发现含 SKILL.md 的子目录。
func scanSkillsUnder(root, scope string, seen map[string]bool, skills *[]Skill) {
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
		if !isScanEntryDir(entryPath, entry) {
			continue
		}

		skillPath := filepath.Join(entryPath, "SKILL.md")
		if content, err := os.ReadFile(skillPath); err == nil {
			if parsed, ok := parseSkillMarkdown(content); ok {
				skillName := parsed.Name
				if skillName == "" {
					skillName = name
				}
				if !seen[skillName] {
					seen[skillName] = true
					*skills = append(*skills, Skill{
						Name:        skillName,
						Description: parsed.Description,
						Location:    skillPath,
						Scope:       scope,
					})
				}
			}
		}

		scanSkillsUnder(entryPath, scope, seen, skills)
	}
}

// isScanEntryDir 判断 skills 根目录下的条目是否为 skill 目录（含指向目录的符号链接）。
func isScanEntryDir(path string, entry os.DirEntry) bool {
	if entry.IsDir() {
		return true
	}
	if entry.Type()&os.ModeSymlink == 0 {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// parseSkillMarkdown 解析 SKILL.md 文件，提取 YAML frontmatter。
func parseSkillMarkdown(content []byte) (skillFrontmatter, bool) {
	text := string(content)

	if !strings.HasPrefix(text, "---") {
		return skillFrontmatter{}, false
	}

	rest := text[3:]
	if strings.HasPrefix(rest, "\r\n") {
		rest = rest[2:]
	} else if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}

	endIdx := strings.Index(rest, "\n---")
	if endIdx < 0 {
		return skillFrontmatter{}, false
	}

	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:endIdx]), &fm); err != nil {
		return skillFrontmatter{}, false
	}
	if fm.Description == "" {
		return skillFrontmatter{}, false
	}

	return fm, true
}
