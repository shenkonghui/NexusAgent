package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"nexusagent/internal/acp"
	"nexusagent/internal/agent"
	"nexusagent/internal/config"
	"nexusagent/internal/database"
	"nexusagent/internal/handlers"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
	"nexusagent/internal/router"
	"nexusagent/internal/services"
)

// ldflags 注入
var version = "dev"

func main() {
	// 命令行参数（桌面客户端模式）
	openBrowser := flag.Bool("open", false, "启动后自动打开浏览器")
	dataDir := flag.String("data-dir", "", "数据目录（覆盖 config.yaml 中的 database.path 和 workspace）")
	showVersion := flag.Bool("version", false, "显示版本号")
	flag.Parse()

	if *showVersion {
		fmt.Printf("NexusAgent %s %s/%s\n", version, runtime.GOOS, runtime.GOARCH)
		return
	}

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

	// --data-dir 覆盖数据库路径与会话工作区
	if *dataDir != "" {
		cfg.Database.Path = filepath.Join(*dataDir, "nexus.db")
		cfg.Agents.Workspace.SessionDir = filepath.Join(*dataDir, "session")
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

	// 从数据库加载用户通过设置页面添加的 agent 配置并注册
	agentCfgRepo := repository.NewAgentConfigRepository(db)
	registrar := agent.NewRegistrar(agentRegistry, acpSvc)
	seedDefaultAgentConfigs(agentCfgRepo)
	loadDBAgentConfigs(agentCfgRepo, registrar)

	agentRouter := agent.NewRouter(agentRegistry, acpSvc)
	agentCfgH := handlers.NewAgentConfigHandler(agentCfgRepo, registrar)

	// 启动健康检查与自动重连 goroutine（定期检查连接状态，断开自动重连）。
	acpSvc.StartHealthCheck()
	// 异步为所有已注册 agent 预建立共享 ACP 连接（每类 agent 一个常驻进程）。
	// 每个 agent 独立 goroutine 连接，不阻塞服务启动；连接失败由健康检查自动重连。
	acpSvc.PreconnectAllAsync()

	// P7: 定时任务调度器
	schedTaskRepo := repository.NewScheduledTaskRepository(db)
	execRepo := repository.NewTaskExecutionRepository(db)
	schedulerSvc := services.NewSchedulerService(schedTaskRepo, execRepo, agentRouter)
	if err := schedulerSvc.Start(); err != nil {
		log.Fatalf("启动定时任务调度器失败: %v", err)
	}
	schedTaskH := handlers.NewScheduledTaskHandler(schedTaskRepo, execRepo, schedulerSvc, agentRouter)

	engine := router.Setup(authSvc, jwtSvc, agentRouter, agentCfgH, schedTaskH, cfg.Server.Mode, cfg.Server.WebDist)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		if cfg.Server.Mode == "release" {
			log.Printf("NexusAgent %s 启动于 %s（单端口模式，前端 + API）", version, addr)
		} else {
			log.Printf("NexusAgent API 启动于 %s（开发模式，前端请访问 vite dev server）", addr)
		}
		if err := engine.Run(addr); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	// 桌面模式：自动打开浏览器
	if *openBrowser {
		go openBrowserAfterDelay(fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port))
	}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("服务正在关闭...")
	schedulerSvc.Stop()
	acpSvc.StopHealthCheck()
}

// openBrowserAfterDelay 延迟打开浏览器（等待服务就绪）
func openBrowserAfterDelay(url string) {
	time.Sleep(800 * time.Millisecond)
	log.Printf("打开浏览器: %s", url)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return
	}
	_ = cmd.Start()
}

// loadDBAgentConfigs 加载数据库中启用的 agent 配置并注册到 registry 与 acp service。
func loadDBAgentConfigs(repo *repository.AgentConfigRepository, registrar *agent.Registrar) {
	list, err := repo.FindAllEnabled()
	if err != nil {
		log.Printf("加载数据库 agent 配置失败: %v", err)
		return
	}
	for i := range list {
		cfg := list[i]
		backend := acp.NewConfigBackend(cfg)
		registrar.ReplaceBackend(backend)
		_ = registrar.RegisterAgent(&agent.AgentDescriptor{
			Type:        cfg.Type,
			DisplayName: cfg.DisplayName,
			Description: cfg.Description,
			Backend:     backend,
		})
		log.Printf("已注册数据库 agent: %s", cfg.Type)
	}
}

// defaultAgentConfigs 是首次启动时写入数据库的默认 agent 配置。
var defaultAgentConfigs = []models.AgentConfig{
	{
		Type:        "claude-code",
		DisplayName: "Claude Code",
		Description: "Anthropic Claude Code 编码 agent",
		Command:     "npx",
		Args:        `["@agentclientprotocol/claude-agent-acp@0.39.0"]`,
		APIKeyEnv:   "ANTHROPIC_API_KEY",
		Timeout:     "300s",
		Enabled:     true,
	},
	{
		Type:        "codebuddy",
		DisplayName: "CodeBuddy",
		Description: "腾讯 CodeBuddy 编码 agent",
		Command:     "npx",
		Args:        `["@tencent-ai/codebuddy-code@2.101.0","--acp"]`,
		Timeout:     "300s",
		Enabled:     true,
	},
	{
		Type:        "kilocode",
		DisplayName: "Kilo Code",
		Description: "Kilo Code 编码 agent",
		Command:     "npx",
		Args:        `["@kilocode/cli@7.3.16","acp"]`,
		Timeout:     "300s",
		Enabled:     true,
	},
	{
		Type:        "devin",
		DisplayName: "Devin",
		Description: "Devin AI agent",
		Command:     "/Users/shenkonghui/.aizen/agents/devin/bin/devin",
		Args:        `["acp"]`,
		Timeout:     "300s",
		Enabled:     true,
	},
}

// seedDefaultAgentConfigs 在 agent_configs 表为空时写入默认配置。
// 已有记录（含用户自定义或禁用的）不会被覆盖。
func seedDefaultAgentConfigs(repo *repository.AgentConfigRepository) {
	existing, err := repo.FindAll()
	if err != nil {
		log.Printf("查询已有 agent 配置失败: %v", err)
		return
	}
	if len(existing) > 0 {
		return // 已有数据，不覆盖
	}
	for i := range defaultAgentConfigs {
		cfg := defaultAgentConfigs[i]
		// 跳过与 config.yaml 内置 claude-code 重复的类型（由内置注册）
		if cfg.Type == "claude-code" {
			// 仍写入 DB 以便设置页统一管理，但内置注册优先
		}
		if err := repo.Create(&cfg); err != nil {
			log.Printf("写入默认 agent 配置 %s 失败: %v", cfg.Type, err)
			continue
		}
		log.Printf("已写入默认 agent 配置: %s", cfg.Type)
	}
}
