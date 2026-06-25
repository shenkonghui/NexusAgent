package router

import (
	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
	"nexusagent/internal/handlers"
	"nexusagent/internal/middleware"
	"nexusagent/internal/services"
)

func Setup(authSvc *services.AuthService, jwtSvc *services.JWTService, agentRouter *agent.Router, agentCfgH *handlers.AgentConfigHandler, schedTaskH *handlers.ScheduledTaskHandler, mode string) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(gin.Recovery())

	authHandler := handlers.NewAuthHandler(authSvc)
	fsHandler := handlers.NewFileSystemHandler()

	v1 := r.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}

		protected := v1.Group("")
		protected.Use(middleware.AuthRequired(jwtSvc))
		{
			protected.GET("/me", authHandler.Me)
			protected.PUT("/me", authHandler.UpdateProfile)
			protected.POST("/me/password", authHandler.ChangePassword)

			agentH := handlers.NewAgentHandler(agentRouter, agentRouter, agentRouter)
			protected.GET("/agents", agentH.List)
			protected.GET("/agents/:type/models", agentH.Models)
			protected.POST("/agents/:type/probe", agentH.Probe)

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
			protected.DELETE("/sessions/:id", sessionH.Close)
			protected.POST("/sessions/:id/delete", sessionH.Delete)
			protected.POST("/sessions/:id/prompt", sessionH.Prompt)
			protected.POST("/sessions/:id/cancel", sessionH.Cancel)
			protected.POST("/sessions/:id/resume", sessionH.Resume)
			protected.GET("/sessions/:id/messages", sessionH.Messages)
			protected.GET("/sessions/:id/commands", sessionH.Commands)
			protected.GET("/sessions/:id/modes", sessionH.Modes)
			protected.GET("/sessions/:id/skills", sessionH.Skills)
			protected.GET("/sessions/:id/config-options", sessionH.ConfigOptions)
			protected.POST("/sessions/:id/config-options", sessionH.SetConfigOption)

			// 会话工作目录文件浏览与编辑（路径限制在 session cwd 内）
			sessionFileH := handlers.NewSessionFileHandler(agentRouter)
			protected.GET("/sessions/:id/files", sessionFileH.ListFiles)
			protected.GET("/sessions/:id/files/content", sessionFileH.ReadFile)
			protected.PUT("/sessions/:id/files/content", sessionFileH.WriteFile)

			// 文件系统目录浏览（用于前端目录选择器）
			protected.GET("/filesystem/dirs", fsHandler.ListDirs)
			protected.GET("/filesystem/list", fsHandler.ListFiles)

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

	return r
}
