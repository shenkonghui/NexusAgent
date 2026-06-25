package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"nexusagent/internal/acp"
	"nexusagent/internal/agent"
	"nexusagent/internal/config"
	"nexusagent/internal/database"
	"nexusagent/internal/router"
	"nexusagent/internal/services"
)

func main() {
	cfgPath := "config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}

	if cfg.Database.Path != ":memory:" {
		dir := filepath.Dir(cfg.Database.Path)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Fatalf("创建数据库目录失败: %v", err)
		}
	}

	db, err := database.Connect(cfg.Database.Path)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	jwtSvc := services.NewJWTService(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	authSvc := services.NewAuthService(db, jwtSvc, cfg.Password.BcryptCost)

	// P1: ACP 服务
	acpSvc := acp.NewService(db, cfg.Agents.Workspace)
	acpSvc.RecoverActiveSessions()
	if cfg.Agents.ClaudeCode.Enabled {
		acpSvc.RegisterBackend(acp.NewClaudeCodeBackend(cfg.Agents.ClaudeCode))
	}

	// P2: Agent 注册表与路由
	agentRegistry := agent.NewRegistry()
	if cfg.Agents.ClaudeCode.Enabled {
		_ = agentRegistry.Register(&agent.AgentDescriptor{
			Type:        "claude-code",
			DisplayName: "Claude Code",
			Description: "Anthropic Claude Code 编码 agent",
			Backend:     acp.NewClaudeCodeBackend(cfg.Agents.ClaudeCode),
		})
	}
	agentRouter := agent.NewRouter(agentRegistry, acpSvc)

	engine := router.Setup(authSvc, jwtSvc, agentRouter, cfg.Server.Mode)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		log.Printf("NexusAgent 认证服务启动于 %s", addr)
		if err := engine.Run(addr); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("服务正在关闭...")
}
