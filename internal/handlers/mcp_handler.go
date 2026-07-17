package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/acp"
)

// MCPHandler 提供全局共享 MCP 配置文件（标准 mcpServers JSON 格式）的读写能力。
type MCPHandler struct {
	configPath string
}

// NewMCPHandler 创建 MCPHandler。configPath 为 mcp.json 绝对路径。
func NewMCPHandler(configPath string) *MCPHandler {
	return &MCPHandler{configPath: configPath}
}

type mcpConfigResponse struct {
	Config string `json:"config"` // 文件原始文本（便于前端直接编辑）
	Path   string `json:"path"`   // 配置文件绝对路径
	Count  int    `json:"count"`  // 解析到的 server 数量
}

type mcpConfigRequest struct {
	Config string `json:"config"`
}

// GetMCPConfig GET /api/v1/config/mcp
// 返回 mcp.json 原始内容；文件不存在时返回空串（前端可据此显示空配置）。
func (h *MCPHandler) GetMCPConfig(c *gin.Context) {
	data, err := os.ReadFile(h.configPath)
	if err != nil {
		if os.IsNotExist(err) {
			Success(c, http.StatusOK, mcpConfigResponse{Config: "", Path: h.configPath, Count: 0})
			return
		}
		Fail(c, http.StatusInternalServerError, "MCP_READ_ERROR", "读取 MCP 配置文件失败")
		return
	}
	Success(c, http.StatusOK, mcpConfigResponse{
		Config: string(data),
		Path:   h.configPath,
		Count:  countMCPServersFromBytes(data),
	})
}

// UpdateMCPConfig PUT /api/v1/config/mcp
// 校验 JSON 合法后写回 mcp.json（保存即生效，新建会话自动注入）。
func (h *MCPHandler) UpdateMCPConfig(c *gin.Context) {
	var req mcpConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_JSON", "请求参数格式错误")
		return
	}
	// 校验 JSON 合法性（空串等价于空配置，允许保存）
	trimmed := req.Config
	if trimmed != "" {
		var probe map[string]any
		if err := json.Unmarshal([]byte(trimmed), &probe); err != nil {
			Fail(c, http.StatusBadRequest, "INVALID_MCP_JSON", "MCP 配置不是合法的 JSON: "+err.Error())
			return
		}
	}
	// 确保父目录存在
	if dir := filepath.Dir(h.configPath); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			Fail(c, http.StatusInternalServerError, "MCP_DIR_ERROR", "创建 MCP 配置目录失败")
			return
		}
	}
	if err := os.WriteFile(h.configPath, []byte(trimmed), 0o644); err != nil {
		Fail(c, http.StatusInternalServerError, "MCP_WRITE_ERROR", "写入 MCP 配置文件失败")
		return
	}
	Success(c, http.StatusOK, mcpConfigResponse{
		Config: trimmed,
		Path:   h.configPath,
		Count:  countMCPServersFromBytes([]byte(trimmed)),
	})
}

// countMCPServersFromBytes 解析 JSON 并返回 mcpServers 数量；非法或无则返回 0。
func countMCPServersFromBytes(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	var file struct {
		McpServers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return 0
	}
	return len(file.McpServers)
}

// GetMCPStatus GET /api/v1/config/mcp/status
// 探测 mcp.json 中配置的所有 MCP server 的连接状态与工具列表。
// 并发探测，每个 server 独立超时；整体兜底超时 60s。
func (h *MCPHandler) GetMCPStatus(c *gin.Context) {
	entries, err := acp.LoadMCPServerEntries(h.configPath)
	if err != nil {
		// 文件不存在或解析失败时返回空列表（而非错误），前端显示"未配置"。
		Success(c, http.StatusOK, gin.H{"servers": []any{}, "error": err.Error()})
		return
	}
	if len(entries) == 0 {
		Success(c, http.StatusOK, gin.H{"servers": []any{}})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()
	servers := acp.ProbeMCPServers(ctx, entries)
	Success(c, http.StatusOK, gin.H{"servers": servers})
}
