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
	authSvc.SeedAdminUser()

	// P1: ACP 服务
	acpSvc := acp.NewService(db, cfg.Agents.Workspace)
	acpSvc.RecoverActiveSessions()
	if cfg.Agents.ClaudeCode.Enabled {
		acpSvc.RegisterBackend(acp.NewClaudeCodeBackend(cfg.Agents.ClaudeCode))
	}

	// P2: Agent 注册表与路由
	agentRegistry := agent.NewRegistry()
	agentCfgRepo := repository.NewAgentConfigRepository(db)
	registrar := agent.NewRegistrar(agentRegistry, acpSvc)

	// 1. config.yaml 内置的 Claude Code
	if cfg.Agents.ClaudeCode.Enabled {
		backend := acp.NewClaudeCodeBackend(cfg.Agents.ClaudeCode)
		acpSvc.RegisterBackend(backend)
		_ = agentRegistry.Register(&agent.AgentDescriptor{
			Type:        "claude-code",
			DisplayName: "Claude Code",
			Description: "Anthropic Claude Code 编码 agent",
			Backend:     backend,
		})
	}

	// 2. 从 ACP registry JSON 加载所有 agent（同步到 DB 并注册）
	loadRegistryAgents(agentRegistry, acpSvc, agentCfgRepo)

	// 3. 从数据库加载用户自定义的 agent 配置
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

	engine := router.Setup(authSvc, jwtSvc, agentRouter, agentCfgH, schedTaskH, cfg.Server.Mode, cfg.Server.WebDist, cfg.Auth.AutoLogin)

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

// openBrowserAfterDelay 延迟后用系统默认浏览器打开 URL（开发模式快速预览）。
// 生产桌面客户端使用 Pake (Tauri) 打包，不依赖此函数。
func openBrowserAfterDelay(url string) {
	time.Sleep(800 * time.Millisecond)
	log.Printf("打开浏览器: %s", url)
	openDefaultBrowser(url)
}

// openDefaultBrowser 用系统默认浏览器打开 URL。
func openDefaultBrowser(url string) {
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

// loadRegistryAgents 从内嵌的 registry.json 加载所有 agent，写入数据库并注册到内存。
func loadRegistryAgents(agentRegistry *agent.Registry, acpSvc *acp.Service, repo *repository.AgentConfigRepository) {
	agents, err := acp.FetchEmbeddedRegistry()
	if err != nil {
		log.Printf("加载内嵌 registry 失败: %v", err)
		return
	}
	log.Printf("加载到 %d 个 agent", len(agents))

	configs := acp.RegistryToAgentConfigs(agents)
	for _, cfg := range configs {
		// 写入数据库（存在则更新 command/args 保持与 registry 同步，不存在则创建并默认禁用）
		existing, _ := repo.FindByType(cfg.Type)
		if existing != nil {
			existing.Command = cfg.Command
			existing.Args = cfg.Args
			existing.DisplayName = cfg.DisplayName
			existing.Description = cfg.Description
			if err := repo.Update(existing); err != nil {
				log.Printf("更新 registry agent %s 失败: %v", cfg.Type, err)
			}
		} else {
			enabled := false
			cfg.Enabled = &enabled
			if err := repo.Create(&cfg); err != nil {
				log.Printf("写入 registry agent %s 失败: %v", cfg.Type, err)
				continue
			}
		}
	}
	// 不在此处注册 backend——统一交由 loadDBAgentConfigs 根据 DB 中的 enabled 状态注册
	log.Printf("registry agent 已同步到数据库（%d 个），等待用户在设置中启用", len(agents))
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
