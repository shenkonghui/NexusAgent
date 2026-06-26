package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Password PasswordConfig `yaml:"password"`
	Agents   AgentsConfig   `yaml:"agents"`
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
	ClaudeCode ClaudeCodeConfig `yaml:"claude_code"`
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
	if v := os.Getenv("CLAUDE_CODE_COMMAND"); v != "" {
		c.Agents.ClaudeCode.Command = v
	}
	if v := os.Getenv("WEB_DIST"); v != "" {
		c.Server.WebDist = v
	}
	if v := os.Getenv("SERVER_MODE"); v != "" {
		c.Server.Mode = v
	}
}

func (c *Config) Validate() error {
	if c.Server.Mode == "" {
		c.Server.Mode = "debug"
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
	return nil
}
