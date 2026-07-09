package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/acp"
	"nexusagent/internal/agent"
	"nexusagent/internal/database"
	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

// recordingRegistrar 记录注册/注销操作以便断言。
type recordingRegistrar struct {
	registered   []string
	replaced     []string
	unregistered []string
}

func (r *recordingRegistrar) RegisterBackend(acp.Backend) {
	r.registered = append(r.registered, "backend")
}
func (r *recordingRegistrar) ReplaceBackend(b acp.Backend) { r.replaced = append(r.replaced, b.Name()) }
func (r *recordingRegistrar) UnregisterBackend(name string) {
	r.unregistered = append(r.unregistered, name)
}
func (r *recordingRegistrar) RegisterAgent(*agent.AgentDescriptor) error {
	r.registered = append(r.registered, "agent")
	return nil
}
func (r *recordingRegistrar) ReplaceAgent(*agent.AgentDescriptor) {
	r.replaced = append(r.replaced, "agent")
}
func (r *recordingRegistrar) UnregisterAgent(agentType string) {
	r.unregistered = append(r.unregistered, agentType)
}

func newAgentConfigTestRouter(t *testing.T) (*gin.Engine, *repository.AgentConfigRepository, *recordingRegistrar) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect("file::agentcfg?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM agent_configs")
	repo := repository.NewAgentConfigRepository(db)
	reg := &recordingRegistrar{}
	h := NewAgentConfigHandler(repo, reg)
	r := gin.New()
	g := r.Group("/api/v1/agent-configs")
	g.GET("", h.List)
	g.POST("", h.Create)
	g.PUT("/:id", h.Update)
	g.DELETE("/:id", h.Delete)
	return r, repo, reg
}

func TestAgentConfigHandler_Create_And_List(t *testing.T) {
	r, _, reg := newAgentConfigTestRouter(t)

	w := doJSON(t, r, "POST", "/api/v1/agent-configs", gin.H{
		"type": "codebuddy", "display_name": "CodeBuddy", "command": "codebuddy",
		"args": []string{"--acp"}, "timeout": "120s",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("状态码 = %d, 期望 201, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data agentConfigItem `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Data.Type != "codebuddy" || resp.Data.Command != "codebuddy" {
		t.Errorf("创建结果字段不正确: %+v", resp.Data)
	}
	if len(resp.Data.Args) != 1 || resp.Data.Args[0] != "--acp" {
		t.Errorf("args 解析不正确: %+v", resp.Data.Args)
	}
	if len(reg.replaced) == 0 {
		t.Error("期望动态注册到 registrar")
	}

	// 列表
	w2 := doJSON(t, r, "GET", "/api/v1/agent-configs", nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", w2.Code)
	}
	var listResp struct {
		Data struct {
			AgentConfigs []agentConfigItem `json:"agent_configs"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &listResp)
	if len(listResp.Data.AgentConfigs) != 1 {
		t.Errorf("列表数量 = %d, 期望 1", len(listResp.Data.AgentConfigs))
	}
}

func TestAgentConfigHandler_Create_WithEnv(t *testing.T) {
	r, _, _ := newAgentConfigTestRouter(t)

	w := doJSON(t, r, "POST", "/api/v1/agent-configs", gin.H{
		"type": "codebuddy", "display_name": "CodeBuddy", "command": "codebuddy",
		"env": map[string]string{
			"HTTPS_PROXY": "http://127.0.0.1:7890",
			"NO_PROXY":    "localhost,127.0.0.1",
		},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("状态码 = %d, 期望 201, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data agentConfigItem `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if resp.Data.Env["HTTPS_PROXY"] != "http://127.0.0.1:7890" {
		t.Errorf("env 往返不正确: %+v", resp.Data.Env)
	}
	if resp.Data.Env["NO_PROXY"] != "localhost,127.0.0.1" {
		t.Errorf("env NO_PROXY 往返不正确: %+v", resp.Data.Env)
	}
}

func TestAgentConfigHandler_Create_DuplicateType(t *testing.T) {
	r, repo, _ := newAgentConfigTestRouter(t)
	_ = repo.Create(newAgentConfig("devin", "Devin", "devin"))

	w := doJSON(t, r, "POST", "/api/v1/agent-configs", gin.H{
		"type": "devin", "display_name": "Devin2", "command": "devin",
	})
	if w.Code != http.StatusConflict {
		t.Fatalf("状态码 = %d, 期望 409, body=%s", w.Code, w.Body.String())
	}
}

func TestAgentConfigHandler_Create_InvalidTimeout(t *testing.T) {
	r, _, _ := newAgentConfigTestRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/agent-configs", gin.H{
		"type": "x", "display_name": "X", "command": "x", "timeout": "not-a-duration",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

func TestAgentConfigHandler_Update_And_Delete(t *testing.T) {
	r, repo, reg := newAgentConfigTestRouter(t)
	cfg := newAgentConfig("claude", "Claude", "claude-code")
	_ = repo.Create(cfg)

	// 更新
	w := doJSON(t, r, "PUT", "/api/v1/agent-configs/"+strconv.Itoa(int(cfg.ID)), gin.H{
		"type": "claude", "display_name": "Claude Code", "command": "claude", "enabled": false,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	// 禁用应触发注销
	if len(reg.unregistered) == 0 {
		t.Error("期望禁用时注销 agent")
	}

	// 删除
	w2 := doJSON(t, r, "DELETE", "/api/v1/agent-configs/"+strconv.Itoa(int(cfg.ID)), nil)
	if w2.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w2.Code, w2.Body.String())
	}
	list, _ := repo.FindAll()
	if len(list) != 0 {
		t.Errorf("删除后列表数量 = %d, 期望 0", len(list))
	}
}

func TestAgentConfigHandler_Update_NotFound(t *testing.T) {
	r, _, _ := newAgentConfigTestRouter(t)
	w := doJSON(t, r, "PUT", "/api/v1/agent-configs/999", gin.H{
		"type": "x", "display_name": "X", "command": "x",
	})
	if w.Code != http.StatusNotFound {
		t.Fatalf("状态码 = %d, 期望 404", w.Code)
	}
}

// newAgentConfig 构造一个启用的 AgentConfig 模型用于测试。
func newAgentConfig(typ, name, command string) *models.AgentConfig {
	enabled := true
	return &models.AgentConfig{
		Type:        typ,
		DisplayName: name,
		Command:     command,
		Enabled:     &enabled,
	}
}
