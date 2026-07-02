package router

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
	"nexusagent/internal/config"
	"nexusagent/internal/handlers"
	"nexusagent/internal/middleware"
	"nexusagent/internal/services"
)

func Setup(authSvc *services.AuthService, jwtSvc *services.JWTService, agentRouter *agent.Router, agentCfgH *handlers.AgentConfigHandler, schedTaskH *handlers.ScheduledTaskHandler, noteH *handlers.NoteHandler, configH *handlers.ConfigHandler, skillsCfg config.SkillsConfig, commandsCfg config.CommandsConfig, rulesCfg config.RulesConfig, mode, webDist string, autoLogin bool) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(gin.Recovery())

	authHandler := handlers.NewAuthHandler(authSvc, autoLogin)
	fsHandler := handlers.NewFileSystemHandler(skillsCfg, commandsCfg, rulesCfg)

	v1 := r.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.GET("/auto-login", authHandler.AutoLogin)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}

		protected := v1.Group("")
		protected.Use(middleware.AuthRequired(jwtSvc))
		{
			protected.GET("/me", authHandler.Me)
			protected.PUT("/me", authHandler.UpdateProfile)
			protected.POST("/me/password", authHandler.ChangePassword)

			agentH := handlers.NewAgentHandler(agentRouter, agentRouter, agentRouter, agentRouter)
			protected.GET("/agents", agentH.List)
			protected.GET("/agents/status", agentH.Status)
			protected.GET("/agents/:type/models", agentH.Models)
			protected.GET("/agents/:type/commands", agentH.Commands)
			protected.GET("/agents/:type/modes", agentH.Modes)
			protected.POST("/agents/:type/probe", agentH.Probe)
			protected.POST("/agents/:type/preconnect", agentH.Preconnect)

			agentCfg := protected.Group("/agent-configs")
			{
				agentCfg.GET("", agentCfgH.List)
				agentCfg.POST("", agentCfgH.Create)
				agentCfg.PUT("/:id", agentCfgH.Update)
				agentCfg.DELETE("/:id", agentCfgH.Delete)
			}

			sessionH := handlers.NewSessionHandler(agentRouter)
			protected.POST("/sessions", sessionH.Create)
			protected.GET("/sessions", sessionH.List)
			protected.GET("/sessions/:id", sessionH.Get)
			protected.PUT("/sessions/:id/title", sessionH.UpdateTitle)
			protected.DELETE("/sessions/:id", sessionH.Delete)
			protected.POST("/sessions/:id/prompt", sessionH.Prompt)
			protected.POST("/sessions/:id/cancel", sessionH.Cancel)
			protected.POST("/sessions/:id/resume", sessionH.Resume)
			protected.GET("/sessions/:id/messages", sessionH.Messages)
			protected.GET("/sessions/:id/executions", sessionH.Executions)
			protected.GET("/sessions/:id/commands", sessionH.Commands)
			protected.GET("/sessions/:id/modes", sessionH.Modes)
			protected.GET("/sessions/:id/skills", sessionH.Skills)
			protected.GET("/sessions/:id/config-options", sessionH.ConfigOptions)
			protected.POST("/sessions/:id/config-options", sessionH.SetConfigOption)
			protected.POST("/sessions/:id/mode", sessionH.SetMode)
			protected.POST("/sessions/:id/permissions/:requestId/respond", sessionH.RespondPermission)

			// Workspace 路由
			workspaceH := handlers.NewWorkspaceHandler(agentRouter)
			protected.POST("/workspaces", workspaceH.Create)
			protected.GET("/workspaces", workspaceH.List)
			protected.GET("/workspaces/:id", workspaceH.Get)
			protected.PUT("/workspaces/:id", workspaceH.Update)
			protected.DELETE("/workspaces/:id", workspaceH.Delete)
			protected.POST("/workspaces/:id/save", workspaceH.Save)

			// 会话工作目录文件浏览与编辑（路径限制在 session cwd 内）
			sessionFileH := handlers.NewSessionFileHandler(agentRouter)
			protected.GET("/sessions/:id/files", sessionFileH.ListFiles)
			protected.GET("/sessions/:id/files/content", sessionFileH.ReadFile)
			protected.PUT("/sessions/:id/files/content", sessionFileH.WriteFile)

			// 配置管理（config.yaml agents 块下的 skills/commands/rules 配置）
			configG := protected.Group("/config")
			{
				configG.GET("/agents", configH.GetAgentsConfig)
				configG.PUT("/agents", configH.UpdateAgentsConfig)
			}

			// 文件系统目录浏览（用于前端目录选择器）
			protected.GET("/filesystem/dirs", fsHandler.ListDirs)
			protected.GET("/filesystem/list", fsHandler.ListFiles)
			protected.GET("/filesystem/skills", fsHandler.Skills)
			protected.GET("/filesystem/commands", fsHandler.Commands)
			protected.GET("/filesystem/rules", fsHandler.Rules)
			protected.GET("/filesystem/file", fsHandler.ReadFile)
			protected.PUT("/filesystem/file", fsHandler.WriteFile)

			// 定时任务
			sched := protected.Group("/scheduled-tasks")
			{
				sched.POST("", schedTaskH.Create)
				sched.GET("", schedTaskH.List)
				sched.GET("/:id", schedTaskH.Get)
				sched.PUT("/:id", schedTaskH.Update)
				sched.DELETE("/:id", schedTaskH.Delete)
				sched.POST("/:id/run", schedTaskH.Run)
				sched.GET("/:id/executions", schedTaskH.Executions)
			}

			// 全局笔记
			notes := protected.Group("/notes")
			{
				notes.GET("/tags", noteH.ListTags)
				notes.GET("/settings", noteH.GetSettings)
				notes.PUT("/settings", noteH.UpdateSettings)
				notes.POST("", noteH.Create)
				notes.GET("", noteH.List)
				notes.GET("/:id", noteH.Get)
				notes.PUT("/:id", noteH.Update)
				notes.DELETE("/:id", noteH.Delete)
			}
		}

		// 终端 WebSocket（通过 query token 认证，不走 AuthRequired 中间件）
		terminalH := handlers.NewTerminalHandler(agentRouter, jwtSvc)
		v1.GET("/sessions/:id/terminal", terminalH.HandleTerminal)
	}

	health := r.Group("/health")
	{
		health.GET("", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})
	}

	// 生产模式（release）下由后端服务前端静态文件，实现单端口部署。
	// debug 模式保留双端口 + vite proxy，便于前端热更新。
	log.Printf("[debug] gin.Mode()=%q webDist=%q", gin.Mode(), webDist)
	if gin.Mode() == gin.ReleaseMode {
		setupStatic(r, webDist)
	}

	return r
}

// setupStatic 让 Gin 服务前端 SPA 静态文件：
//   - 已存在的静态资源（如 /assets/*.js）直接返回
//   - 非 /api 的未匹配路径返回 index.html，由前端路由处理（BrowserRouter SPA fallback）
//   - /api 下未匹配路径返回 404 JSON，避免把 API 请求误判为前端路由
func setupStatic(r *gin.Engine, webDist string) {
	if webDist == "" {
		log.Printf("[setupStatic] webDist is empty, skipping")
		return
	}
	if info, err := os.Stat(webDist); err != nil || !info.IsDir() {
		log.Printf("[setupStatic] webDist %q stat error: %v isDir=%v, skipping", webDist, err, info != nil && info.IsDir())
		return
	}
	indexFile := filepath.Join(webDist, "index.html")
	log.Printf("[setupStatic] serving static from %q, index=%q", webDist, indexFile)
	fileServer := http.FileServer(http.Dir(webDist))

	r.NoRoute(func(c *gin.Context) {
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		// 尝试服务真实存在的静态文件
		cleanPath := filepath.Join(webDist, filepath.Clean("/"+path))
		if info, err := os.Stat(cleanPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}
		// SPA fallback：返回 index.html，交给前端路由
		c.Header("Cache-Control", "no-cache")
		c.File(indexFile)
	})
}
