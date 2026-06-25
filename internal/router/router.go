package router

import (
	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
	"nexusagent/internal/handlers"
	"nexusagent/internal/middleware"
	"nexusagent/internal/services"
)

func Setup(authSvc *services.AuthService, jwtSvc *services.JWTService, agentRouter *agent.Router, mode string) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(gin.Recovery())

	authHandler := handlers.NewAuthHandler(authSvc)

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

			agentH := handlers.NewAgentHandler(agentRouter)
			protected.GET("/agents", agentH.List)

			sessionH := handlers.NewSessionHandler(agentRouter)
			protected.POST("/sessions", sessionH.Create)
			protected.GET("/sessions", sessionH.List)
			protected.GET("/sessions/:id", sessionH.Get)
			protected.DELETE("/sessions/:id", sessionH.Close)
			protected.POST("/sessions/:id/prompt", sessionH.Prompt)
			protected.POST("/sessions/:id/cancel", sessionH.Cancel)
			protected.POST("/sessions/:id/resume", sessionH.Resume)
			protected.GET("/sessions/:id/messages", sessionH.Messages)
		}
	}

	health := r.Group("/health")
	{
		health.GET("", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})
	}

	return r
}
