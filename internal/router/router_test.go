package router

import (
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
	"nexusagent/internal/database"
	"nexusagent/internal/services"
)

func TestSetup_RegistersP5Routes(t *testing.T) {
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	jwtSvc := services.NewJWTService("this-is-a-very-long-jwt-secret-key-32+bytes!", 15*time.Minute, time.Hour)
	authSvc := services.NewAuthService(db, jwtSvc, 10)
	agentRouter := agent.NewRouter(agent.NewRegistry(), nil)

	engine := Setup(authSvc, jwtSvc, agentRouter, gin.TestMode)

	want := []string{
		"GET /api/v1/agents",
		"POST /api/v1/sessions",
		"GET /api/v1/sessions",
		"GET /api/v1/sessions/:id",
		"DELETE /api/v1/sessions/:id",
		"POST /api/v1/sessions/:id/prompt",
		"POST /api/v1/sessions/:id/cancel",
		"POST /api/v1/sessions/:id/resume",
		"GET /api/v1/sessions/:id/messages",
	}
	got := make(map[string]bool)
	for _, ri := range engine.Routes() {
		got[ri.Method+" "+ri.Path] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("缺少路由 %s", w)
		}
	}
}
