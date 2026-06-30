package acp

import (
	"os"
	"path/filepath"
	"strings"

	"nexusagent/internal/config"
)

type scanDirEntry struct {
	Path  string
	Scope string // "project" | "user"
}

var defaultSkillProjectDirs = []string{".agents/skills", ".claude/skills"}
var defaultSkillUserDirs = []string{".agents/skills", ".claude/skills"}
var defaultCommandProjectDirs = []string{".cursor/commands", ".claude/commands"}
var defaultCommandUserDirs = []string{".cursor/commands", ".claude/commands"}

func skillScanEntries(cwd, home string, cfg config.SkillsConfig) []scanDirEntry {
	return buildScanEntries(cwd, home, cfg.ProjectDirs, cfg.UserDirs, defaultSkillProjectDirs, defaultSkillUserDirs)
}

func commandScanEntries(cwd, home string, cfg config.SlashCommandsConfig) []scanDirEntry {
	return buildScanEntries(cwd, home, cfg.ProjectDirs, cfg.UserDirs, defaultCommandProjectDirs, defaultCommandUserDirs)
}

func buildScanEntries(cwd, home string, projectDirs, userDirs, defaultProject, defaultUser []string) []scanDirEntry {
	if len(projectDirs) == 0 {
		projectDirs = defaultProject
	}
	if len(userDirs) == 0 {
		userDirs = defaultUser
	}

	var entries []scanDirEntry
	if cwd != "" {
		for _, rel := range projectDirs {
			if p := resolveScanPath(cwd, rel); p != "" {
				entries = append(entries, scanDirEntry{Path: p, Scope: "project"})
			}
		}
	}
	if home != "" {
		for _, rel := range userDirs {
			if p := resolveScanPath(home, rel); p != "" {
				entries = append(entries, scanDirEntry{Path: p, Scope: "user"})
			}
		}
	}
	return entries
}

func resolveScanPath(base, p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		rest := strings.TrimPrefix(p, "~")
		rest = strings.TrimPrefix(rest, "/")
		if rest == "" {
			return filepath.Clean(home)
		}
		return filepath.Clean(filepath.Join(home, rest))
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	return filepath.Clean(filepath.Join(base, p))
}

func isHiddenEntry(name string) bool {
	return strings.HasPrefix(name, ".")
}
