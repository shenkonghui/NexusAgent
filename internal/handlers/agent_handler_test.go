package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/agent"
)

type fakeAgentLister struct{ descs []*agent.AgentDescriptor }

func (f *fakeAgentLister) ListAgents() []*agent.AgentDescriptor { return f.descs }

func newAgentTestRouter(lister AgentLister) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentHandler(lister, nil, nil, nil)
	r.GET("/api/v1/agents", h.List)
	return r
}

func TestAgentHandler_List_Empty(t *testing.T) {
	r := newAgentTestRouter(&fakeAgentLister{descs: nil})
	w := doJSON(t, r, "GET", "/api/v1/agents", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Agents []agentItem `json:"agents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v, body=%s", err, w.Body.String())
	}
	if len(resp.Data.Agents) != 0 {
		t.Fatalf("agents 数量 = %d, 期望 0", len(resp.Data.Agents))
	}
}

func TestAgentHandler_List_WithAgents(t *testing.T) {
	descs := []*agent.AgentDescriptor{
		{Type: "claude-code", DisplayName: "Claude Code", Description: "Anthropic Claude Code"},
		{Type: "codex", DisplayName: "Codex", Description: "OpenAI Codex"},
	}
	r := newAgentTestRouter(&fakeAgentLister{descs: descs})
	w := doJSON(t, r, "GET", "/api/v1/agents", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			Agents []agentItem `json:"agents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("解析响应失败: %v", err)
	}
	if len(resp.Data.Agents) != 2 {
		t.Fatalf("agents 数量 = %d, 期望 2", len(resp.Data.Agents))
	}
	first := resp.Data.Agents[0]
	if first.Type != "claude-code" || first.DisplayName != "Claude Code" || first.Description != "Anthropic Claude Code" {
		t.Errorf("首个 agent 字段不正确: %+v", first)
	}
}
