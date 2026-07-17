package acp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// startTestMCPServer 启动一个本地 streamable HTTP MCP server（带 1 个测试 tool），
// 返回其 endpoint URL 与关闭函数。
func startTestMCPServer(t *testing.T) (string, func()) {
	t.Helper()
	handler := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		srv := mcp.NewServer(&mcp.Implementation{Name: "test-mcp-server", Version: "1.0.0"}, nil)
		mcp.AddTool(srv, &mcp.Tool{
			Name:        "echo",
			Description: "echo back the input",
		}, func(ctx context.Context, _ *mcp.CallToolRequest, in struct{ Msg string `json:"msg"` }) (*mcp.CallToolResult, struct{ Reply string `json:"reply"` }, error) {
			return nil, struct{ Reply string `json:"reply"` }{Reply: in.Msg}, nil
		})
		return srv
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	ts := httptest.NewServer(handler)
	return ts.URL, ts.Close
}

func TestProbeMCPServers_HTTPConnected(t *testing.T) {
	url, closeFn := startTestMCPServer(t)
	defer closeFn()

	entries := []NamedMCPServerEntry{
		{Name: "test-http", Entry: MCPServerEntry{Type: MCPTypeHTTP, Url: url}},
	}
	statuses := ProbeMCPServers(context.Background(), entries)
	if len(statuses) != 1 {
		t.Fatalf("期望 1 个状态，实际 %d", len(statuses))
	}
	s := statuses[0]
	if !s.Connected {
		t.Fatalf("期望已连接，实际失败: %s", s.Error)
	}
	if s.ServerInfo != "test-mcp-server" {
		t.Errorf("ServerInfo = %q, 期望 test-mcp-server", s.ServerInfo)
	}
	// 应探测到 echo 工具
	if len(s.Tools) != 1 {
		t.Fatalf("期望 1 个工具，实际 %d", len(s.Tools))
	}
	if s.Tools[0].Name != "echo" {
		t.Errorf("Tools[0].Name = %q, 期望 echo", s.Tools[0].Name)
	}
	if s.Tools[0].Description != "echo back the input" {
		t.Errorf("Tools[0].Description = %q", s.Tools[0].Description)
	}
}

func TestProbeMCPServers_HTTPUnreachable(t *testing.T) {
	// 探测一个不存在的端口，应标记为连接失败
	entries := []NamedMCPServerEntry{
		{Name: "dead-http", Entry: MCPServerEntry{Type: MCPTypeHTTP, Url: "http://127.0.0.1:1/mcp"}},
	}
	statuses := ProbeMCPServers(context.Background(), entries)
	if len(statuses) != 1 {
		t.Fatalf("期望 1 个状态，实际 %d", len(statuses))
	}
	s := statuses[0]
	if s.Connected {
		t.Error("期望连接失败，实际已连接")
	}
	if s.Error == "" {
		t.Error("期望 Error 非空")
	}
}

func TestProbeMCPServers_InvalidConfig(t *testing.T) {
	// stdio 缺 command：应直接标记失败，不发起探测
	entries := []NamedMCPServerEntry{
		{Name: "bad-stdio", Entry: MCPServerEntry{Type: MCPTypeStdio}},
	}
	statuses := ProbeMCPServers(context.Background(), entries)
	if len(statuses) != 1 {
		t.Fatalf("期望 1 个状态，实际 %d", len(statuses))
	}
	s := statuses[0]
	if s.Connected {
		t.Error("期望配置非法时连接失败")
	}
	if s.Error == "" {
		t.Error("期望 Error 描述非法原因")
	}
	if s.Type != MCPTypeStdio {
		t.Errorf("Type = %q, 期望 stdio", s.Type)
	}
}

func TestProbeMCPServers_Empty(t *testing.T) {
	if got := ProbeMCPServers(context.Background(), nil); got != nil {
		t.Errorf("空输入应返回 nil，实际 %v", got)
	}
}

func TestProbeMCPServers_Order(t *testing.T) {
	// 结果顺序应与输入顺序一致
	url, closeFn := startTestMCPServer(t)
	defer closeFn()
	entries := []NamedMCPServerEntry{
		{Name: "zeta", Entry: MCPServerEntry{Type: MCPTypeHTTP, Url: url}},
		{Name: "alpha", Entry: MCPServerEntry{Type: MCPTypeHTTP, Url: "http://127.0.0.1:1/x"}},
	}
	statuses := ProbeMCPServers(context.Background(), entries)
	if len(statuses) != 2 {
		t.Fatalf("期望 2 个状态，实际 %d", len(statuses))
	}
	if statuses[0].Name != "zeta" {
		t.Errorf("statuses[0].Name = %q, 期望 zeta", statuses[0].Name)
	}
	if statuses[1].Name != "alpha" {
		t.Errorf("statuses[1].Name = %q, 期望 alpha", statuses[1].Name)
	}
}

func TestHeaderClient_InjectsHeaders(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		// 返回简单响应，让请求完成（内容不重要，只验证 header 注入）
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	client := headerClient(map[string]string{"Authorization": "Bearer test-token"})
	req, _ := http.NewRequest("GET", ts.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}
	defer resp.Body.Close()
	if gotAuth != "Bearer test-token" {
		t.Errorf("Authorization header = %q, 期望 Bearer test-token", gotAuth)
	}
}

func TestHeaderClient_NoHeaders(t *testing.T) {
	// 无 headers 时应返回 http.DefaultClient
	if c := headerClient(nil); c != http.DefaultClient {
		t.Error("无 headers 时应返回 DefaultClient")
	}
	if c := headerClient(map[string]string{}); c != http.DefaultClient {
		t.Error("空 map 时应返回 DefaultClient")
	}
}

func TestBuildTransport_Stdio(t *testing.T) {
	tr, err := buildTransport(MCPServerEntry{Type: MCPTypeStdio, Command: "echo", Args: []string{"hi"}})
	if err != nil {
		t.Fatalf("buildTransport 错误: %v", err)
	}
	if _, ok := tr.(*mcp.CommandTransport); !ok {
		t.Errorf("期望 *CommandTransport，实际 %T", tr)
	}
}

func TestBuildTransport_SSE(t *testing.T) {
	tr, err := buildTransport(MCPServerEntry{Type: MCPTypeSSE, Url: "http://example.com/sse"})
	if err != nil {
		t.Fatalf("buildTransport 错误: %v", err)
	}
	if s, ok := tr.(*mcp.SSEClientTransport); !ok {
		t.Errorf("期望 *SSEClientTransport，实际 %T", tr)
	} else if s.Endpoint != "http://example.com/sse" {
		t.Errorf("Endpoint = %q", s.Endpoint)
	}
}

func TestBuildTransport_UnknownType(t *testing.T) {
	if _, err := buildTransport(MCPServerEntry{Type: "weird"}); err == nil {
		t.Error("未知 type 应返回错误")
	}
}

func TestNormalizedType(t *testing.T) {
	if got := normalizedType(""); got != MCPTypeStdio {
		t.Errorf("normalizedType('') = %q, 期望 stdio", got)
	}
	if got := normalizedType("http"); got != "http" {
		t.Errorf("normalizedType('http') = %q", got)
	}
}
