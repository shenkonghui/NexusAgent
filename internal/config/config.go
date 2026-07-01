package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Auth     AuthConfig     `yaml:"auth"`
	Password PasswordConfig `yaml:"password"`
	Logging  LoggingConfig  `yaml:"logging"`
	Agents   AgentsConfig   `yaml:"agents"`
}

// LoggingConfig 控制应用与 ACP 交互日志输出。
type LoggingConfig struct {
	// Level 日志等级：debug | info | warn | error。debug 时输出每次 agent 交互详情。
	Level string `yaml:"level"`
}

type AuthConfig struct {
	AutoLogin bool `yaml:"auto_login"`
}

type ServerConfig struct {
	Port    int    `yaml:"port"`
	Mode    string `yaml:"mode"`
	WebDist string `yaml:"web_dist"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type JWTConfig struct {
	Secret     string        `yaml:"secret"`
	AccessTTL  time.Duration `yaml:"access_ttl"`
	RefreshTTL time.Duration `yaml:"refresh_ttl"`
}

type PasswordConfig struct {
	BcryptCost int `yaml:"bcrypt_cost"`
}

type AgentsConfig struct {
	Workspace  WorkspaceConfig  `yaml:"workspace"`
	Skills     SkillsConfig     `yaml:"skills"`
	Commands   CommandsConfig   `yaml:"commands"`
	Rules      RulesConfig      `yaml:"rules"`
	ClaudeCode ClaudeCodeConfig `yaml:"claude_code"`
}

// SkillsConfig 配置 Agent Skills 扫描目录（agentskills.io 规范）。
type SkillsConfig struct {
	// UserDirs 用户级 skills 根目录（绝对路径或 ~/ 开头），默认 ~/.claude/skills。
	UserDirs []string `yaml:"user_dirs"`
	// ProjectDirs 项目级 skills 相对工作区 cwd 的子目录，默认 .claude/skills、.agents/skills。
	ProjectDirs []string `yaml:"project_dirs"`
}

// CommandsConfig 配置 Slash Command 扫描目录（Claude Code 规范：递归 *.md，支持子目录与符号链接）。
type CommandsConfig struct {
	// UserDirs 用户级 commands 根目录，默认 ~/.claude/commands。
	UserDirs []string `yaml:"user_dirs"`
	// ProjectDirs 项目级 commands 相对 cwd 的子目录，默认 .claude/commands。
	ProjectDirs []string `yaml:"project_dirs"`
}

// RulesConfig 配置 Rule 扫描路径（目录或单个文件，如 ~/.claude/CLAUDE.md）。
type RulesConfig struct {
	// UserDirs 用户级 rules 路径（目录或 .md/.mdc 文件），默认 ~/.cursor/rules、~/.claude/CLAUDE.md。
	UserDirs []string `yaml:"user_dirs"`
	// ProjectDirs 项目级 rules 相对 cwd 的路径，默认 .cursor/rules、CLAUDE.md。
	ProjectDirs []string `yaml:"project_dirs"`
}

type WorkspaceConfig struct {
	DefaultMode   string `yaml:"default_mode"`
	TempDirPrefix string `yaml:"temp_dir_prefix"`
	// SessionDir 是 temporary 模式会话工作区的存放根目录。
	// 默认 ~/.nextAgent/session，由程序在删除会话时清理，不依赖系统清理临时目录。
	SessionDir string `yaml:"session_dir"`
}

type ClaudeCodeConfig struct {
	Enabled   bool          `yaml:"enabled"`
	Command   string        `yaml:"command"`
	Args      []string      `yaml:"args"`
	APIKeyEnv string        `yaml:"api_key_env"`
	Timeout   time.Duration `yaml:"timeout"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件: %w", err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件: %w", err)
	}
	cfg.applyEnv()
	return cfg, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWT.Secret = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			c.Server.Port = port
		}
	}
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		c.Database.Path = v
	}
	if v := os.Getenv("AGENTS_WORKSPACE_DEFAULT_MODE"); v != "" {
		c.Agents.Workspace.DefaultMode = v
	}
	if v := os.Getenv("AGENTS_WORKSPACE_SESSION_DIR"); v != "" {
		c.Agents.Workspace.SessionDir = v
	}
	if v := os.Getenv("AGENTS_SKILLS_USER_DIRS"); v != "" {
		c.Agents.Skills.UserDirs = splitCommaList(v)
	}
	if v := os.Getenv("AGENTS_COMMANDS_USER_DIRS"); v != "" {
		c.Agents.Commands.UserDirs = splitCommaList(v)
	}
	if v := os.Getenv("AGENTS_RULES_USER_DIRS"); v != "" {
		c.Agents.Rules.UserDirs = splitCommaList(v)
	}
	if v := os.Getenv("CLAUDE_CODE_COMMAND"); v != "" {
		c.Agents.ClaudeCode.Command = v
	}
	if v := os.Getenv("WEB_DIST"); v != "" {
		c.Server.WebDist = v
	}
	if v := os.Getenv("SERVER_MODE"); v != "" {
		c.Server.Mode = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		c.Logging.Level = v
	}
}

func (c *Config) Validate() error {
	if c.Server.Mode == "" {
		c.Server.Mode = "debug"
	}
	if c.Logging.Level == "" {
		c.Logging.Level = "info"
	}
	if c.Server.WebDist == "" {
		c.Server.WebDist = "./web/dist"
	}
	if c.JWT.AccessTTL <= 0 {
		c.JWT.AccessTTL = 15 * time.Minute
	}
	if c.JWT.RefreshTTL <= 0 {
		c.JWT.RefreshTTL = 168 * time.Hour
	}
	if c.Password.BcryptCost == 0 {
		c.Password.BcryptCost = 12
	}
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("JWT_SECRET 长度必须 >= 32 字节，当前 %d", len(c.JWT.Secret))
	}
	if c.Agents.Workspace.DefaultMode == "" {
		c.Agents.Workspace.DefaultMode = "temporary"
	}
	if c.Agents.Workspace.DefaultMode != "external" && c.Agents.Workspace.DefaultMode != "temporary" {
		return fmt.Errorf("agents.workspace.default_mode 必须是 external 或 temporary，当前 %q", c.Agents.Workspace.DefaultMode)
	}
	if c.Agents.Workspace.TempDirPrefix == "" {
		c.Agents.Workspace.TempDirPrefix = "nexus-"
	}
	if c.Agents.Workspace.SessionDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("获取用户主目录以设置 session_dir: %w", err)
		}
		c.Agents.Workspace.SessionDir = filepath.Join(home, ".nextAgent", "session")
	}
	if c.Agents.ClaudeCode.Command == "" {
		c.Agents.ClaudeCode.Command = "npx"
	}
	if len(c.Agents.ClaudeCode.Args) == 0 {
		c.Agents.ClaudeCode.Args = []string{"-y", "@zed-industries/claude-code-acp@latest"}
	}
	if c.Agents.ClaudeCode.APIKeyEnv == "" {
		c.Agents.ClaudeCode.APIKeyEnv = "ANTHROPIC_API_KEY"
	}
	if c.Agents.ClaudeCode.Timeout <= 0 {
		c.Agents.ClaudeCode.Timeout = 300 * time.Second
	}
	if err := c.Agents.Skills.normalize(); err != nil {
		return err
	}
	if err := c.Agents.Commands.normalize(); err != nil {
		return err
	}
	if err := c.Agents.Rules.normalize(); err != nil {
		return err
	}
	return nil
}

// normalize 填充 skills 默认值并展开路径。
func (s *SkillsConfig) normalize() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录以设置 skills 路径: %w", err)
	}
	if len(s.UserDirs) == 0 {
		s.UserDirs = []string{filepath.Join(home, ".claude", "skills")}
	} else {
		resolved := make([]string, 0, len(s.UserDirs))
		for _, p := range s.UserDirs {
			abs, err := expandPath(p)
			if err != nil {
				return fmt.Errorf("skills.user_dirs 路径 %q 无效: %w", p, err)
			}
			resolved = append(resolved, abs)
		}
		s.UserDirs = resolved
	}
	if len(s.ProjectDirs) == 0 {
		s.ProjectDirs = []string{".claude/skills", ".agents/skills"}
	}
	return nil
}

// normalize 填充 commands 默认值并展开路径（Claude Code：~/.claude/commands、.claude/commands）。
func (c *CommandsConfig) normalize() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录以设置 commands 路径: %w", err)
	}
	if len(c.UserDirs) == 0 {
		c.UserDirs = []string{filepath.Join(home, ".claude", "commands")}
	} else {
		resolved := make([]string, 0, len(c.UserDirs))
		for _, p := range c.UserDirs {
			abs, err := expandPath(p)
			if err != nil {
				return fmt.Errorf("commands.user_dirs 路径 %q 无效: %w", p, err)
			}
			resolved = append(resolved, abs)
		}
		c.UserDirs = resolved
	}
	if len(c.ProjectDirs) == 0 {
		c.ProjectDirs = []string{".claude/commands"}
	}
	return nil
}

// normalize 填充 rules 默认值并展开路径。
func (r *RulesConfig) normalize() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("获取用户主目录以设置 rules 路径: %w", err)
	}
	if len(r.UserDirs) == 0 {
		r.UserDirs = []string{
			filepath.Join(home, ".cursor", "rules"),
			filepath.Join(home, ".claude", "CLAUDE.md"),
		}
	} else {
		resolved := make([]string, 0, len(r.UserDirs))
		for _, p := range r.UserDirs {
			abs, err := expandPath(p)
			if err != nil {
				return fmt.Errorf("rules.user_dirs 路径 %q 无效: %w", p, err)
			}
			resolved = append(resolved, abs)
		}
		r.UserDirs = resolved
	}
	if len(r.ProjectDirs) == 0 {
		r.ProjectDirs = []string{".cursor/rules", "CLAUDE.md"}
	}
	return nil
}

func expandPath(p string) (string, error) {
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return home, nil
	}
	if len(p) >= 2 && p[0] == '~' && (p[1] == '/' || p[1] == '\\') {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Abs(p)
}

func splitCommaList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
