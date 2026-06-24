package router

import (
	"github.com/gin-gonic/gin"

	"nexusagent/internal/handlers"
	"nexusagent/internal/middleware"
	"nexusagent/internal/services"
)

func Setup(authSvc *services.AuthService, jwtSvc *services.JWTService, mode string) *gin.Engine {
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
