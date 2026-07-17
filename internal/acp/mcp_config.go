package acp

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"github.com/coder/acp-go-sdk"
)

// mcpServersFile 是标准 mcpServers JSON 格式的文件结构（Claude Desktop / ZCode 规范）。
type mcpServersFile struct {
	McpServers map[string]MCPServerEntry `json:"mcpServers"`
}

// MCPServerEntry 是单个 MCP server 的原始配置。不同传输类型字段不同，全部可选解析。
//   - stdio（默认或 type=="stdio"）：command / args / env
//   - http（type=="http"）：url / headers
//   - sse（type=="sse"）：url / headers
type MCPServerEntry struct {
	Type    string            `json:"type"`              // stdio | http | sse；空则按 stdio 处理
	Command string            `json:"command,omitempty"` // stdio
	Args    []string          `json:"args,omitempty"`    // stdio
	Env     map[string]string `json:"env,omitempty"`     // stdio
	Url     string            `json:"url,omitempty"`     // http / sse
	Headers map[string]string `json:"headers,omitempty"` // http / sse
}

// NamedMCPServerEntry 是带名称（mcpServers map 的 key）的 server 配置。
type NamedMCPServerEntry struct {
	Name  string
	Entry MCPServerEntry
}

// MCP 传输类型常量。
const (
	MCPTypeStdio = "stdio"
	MCPTypeHTTP  = "http"
	MCPTypeSSE   = "sse"
)

// LoadMCPServerEntries 从 path 读取并解析标准 mcpServers 格式的配置文件，
// 返回带名称的原始 server 配置列表（按 name 字典序排序）。
//
// 容错策略：
//   - 文件不存在：返回 (nil, nil)（不视为错误）
//   - 整体 JSON 非法：返回错误
//
// 注意：此函数返回所有配置项（含缺少 command/url 等无效项），由调用方按需校验。
func LoadMCPServerEntries(path string) ([]NamedMCPServerEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("读取 MCP 配置文件: %w", err)
	}

	var file mcpServersFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("解析 MCP 配置文件: %w", err)
	}
	if len(file.McpServers) == 0 {
		return nil, nil
	}

	// 按 name 排序，保证顺序稳定。
	names := make([]string, 0, len(file.McpServers))
	for name := range file.McpServers {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]NamedMCPServerEntry, 0, len(names))
	for _, name := range names {
		entries = append(entries, NamedMCPServerEntry{Name: name, Entry: file.McpServers[name]})
	}
	return entries, nil
}

// LoadMCPServers 从 path 读取并解析标准 mcpServers 格式的配置文件，
// 转换为 ACP NewSession 所需的 []acp.McpServer。
//
// 容错策略：
//   - 文件不存在：返回 (nil, nil)（不视为错误）
//   - 单条 server 配置非法：跳过并 slog.Warn，不整体失败
//   - 整体 JSON 非法：返回错误
//
// 返回顺序按 server name 字典序，保证稳定。
func LoadMCPServers(path string) ([]acp.McpServer, error) {
	entries, err := LoadMCPServerEntries(path)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	servers := make([]acp.McpServer, 0, len(entries))
	for _, ne := range entries {
		server, ok := convertMcpServer(ne.Name, ne.Entry)
		if !ok {
			continue
		}
		servers = append(servers, server)
	}
	if len(servers) == 0 {
		return nil, nil
	}
	return servers, nil
}

// convertMcpServer 将单条配置转换为 acp.McpServer。配置非法时返回 (zero, false) 并记日志。
func convertMcpServer(name string, e MCPServerEntry) (acp.McpServer, bool) {
	switch e.Type {
	case "", MCPTypeStdio:
		if e.Command == "" {
			slog.Warn("跳过 MCP server：stdio 类型缺少 command", "name", name)
			return acp.McpServer{}, false
		}
		return acp.McpServer{
			Stdio: &acp.McpServerStdio{
				Name:    name,
				Command: e.Command,
				Args:    e.Args,
				Env:     toEnvVariables(e.Env),
			},
		}, true
	case MCPTypeHTTP:
		if e.Url == "" {
			slog.Warn("跳过 MCP server：http 类型缺少 url", "name", name)
			return acp.McpServer{}, false
		}
		return acp.McpServer{
			Http: &acp.McpServerHttpInline{
				Name:    name,
				Type:    MCPTypeHTTP,
				Url:     e.Url,
				Headers: toHttpHeaders(e.Headers),
			},
		}, true
	case MCPTypeSSE:
		if e.Url == "" {
			slog.Warn("跳过 MCP server：sse 类型缺少 url", "name", name)
			return acp.McpServer{}, false
		}
		return acp.McpServer{
			Sse: &acp.McpServerSseInline{
				Name:    name,
				Type:    MCPTypeSSE,
				Url:     e.Url,
				Headers: toHttpHeaders(e.Headers),
			},
		}, true
	default:
		slog.Warn("跳过 MCP server：未知的 type", "name", name, "type", e.Type)
		return acp.McpServer{}, false
	}
}

// toEnvVariables 将 map 转换为按 name 排序的 []EnvVariable（顺序稳定）。
func toEnvVariables(env map[string]string) []acp.EnvVariable {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]acp.EnvVariable, 0, len(keys))
	for _, k := range keys {
		out = append(out, acp.EnvVariable{Name: k, Value: env[k]})
	}
	return out
}

// toHttpHeaders 将 map 转换为按 name 排序的 []HttpHeader（顺序稳定）。
func toHttpHeaders(headers map[string]string) []acp.HttpHeader {
	if len(headers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]acp.HttpHeader, 0, len(keys))
	for _, k := range keys {
		out = append(out, acp.HttpHeader{Name: k, Value: headers[k]})
	}
	return out
}

// CountMCPServers 解析 path 并返回其中配置的 server 数量（含被跳过的非法项）。
// 文件不存在或解析失败返回 0。
func CountMCPServers(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var file mcpServersFile
	if err := json.Unmarshal(data, &file); err != nil {
		return 0
	}
	return len(file.McpServers)
}

// UpsertMCPServerEntry 向 path 的 mcpServers 中插入或更新名为 name 的 server 条目，
// 保留其它已有条目与格式（2 空格缩进）。文件或父目录不存在时自动创建。
//
// 容错策略：
//   - 文件不存在：按空配置起步
//   - 文件内容非法 JSON：返回错误，避免覆盖用户数据
func UpsertMCPServerEntry(path, name string, entry MCPServerEntry) error {
	file := mcpServersFile{McpServers: map[string]MCPServerEntry{}}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &file); err != nil {
			return fmt.Errorf("解析现有 MCP 配置文件失败，已中止写入以保护数据: %w", err)
		}
	}
	if file.McpServers == nil {
		file.McpServers = map[string]MCPServerEntry{}
	}
	file.McpServers[name] = entry

	out, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return fmt.Errorf("编码 MCP 配置失败: %w", err)
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("创建 MCP 配置目录失败: %w", err)
		}
	}
	return os.WriteFile(path, out, 0o644)
}
