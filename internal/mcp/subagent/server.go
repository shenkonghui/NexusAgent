// Package subagentmcp 提供 nexus-subagent MCP server，让主 agent 通过 MCP 工具调起预定义的 subagent。
//
// subagent 定义来自 markdown 文件（~/.agents/agents/*.md 等，由 acp.ScanSubAgents 扫描），
// 不再依赖数据库。frontmatter 含 name/description/model/tools，markdown 正文作为注入会话的 system_prompt。
//
// 暴露 3 个工具：
//   - list_subagents：列出所有 subagent 摘要
//   - get_subagent：  查询单个 subagent 详情
//   - run_subagent：  执行 subagent 任务（一次性模式，阻塞返回文本结果）
//
// 鉴权复用 nexus-notes 的 Bearer token 体系（用户级共享一个 token），用于解析"继承父 agent"。
package subagentmcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"nexusagent/internal/acp"
	"nexusagent/internal/repository"
)

// SubAgentCatalog 提供基于文件扫描的 subagent 发现能力。
// 由 *acp.Service 实现。
type SubAgentCatalog interface {
	ListSubAgents() []acp.SubAgentDef
	ResolveSubAgent(name string) *acp.SubAgentDef
}

// SubAgentRunner 由 *agent.Router 实现，用于执行一次性 subagent 任务。
type SubAgentRunner interface {
	RunSubAgent(ctx context.Context, cfg acp.SubAgentRunConfig) (string, error)
}

// Handler 返回带 Bearer 鉴权的 subagent MCP Streamable HTTP Handler。
//
// prefsRepo 用于解析"继承父 agent"：subagent 文件不指定 agent 后端，运行时取用户最近使用的 agent 类型。
func Handler(catalog SubAgentCatalog, settings *repository.NoteSettingsRepository, prefsRepo *repository.UserAgentPrefsRepository, runner SubAgentRunner) http.Handler {
	inner := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(catalog, prefsRepo, runner)
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, err := BearerUserID(r, settings)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r.WithContext(withUserID(r.Context(), uid)))
	})
}

func newServer(catalog SubAgentCatalog, prefsRepo *repository.UserAgentPrefsRepository, runner SubAgentRunner) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "nexus-subagent", Version: "1.0.0"}, nil)

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_subagents",
		Description: "列出所有 subagent 摘要，用于决定调用哪个 subagent",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, listSubAgentsOut, error) {
		return handleListSubAgents(ctx, catalog)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_subagent",
		Description: "按 name 获取 subagent 的详细定义（包含 description/model/tools 等），用于决定如何调用",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in getSubAgentIn) (*mcp.CallToolResult, getSubAgentOut, error) {
		return handleGetSubAgent(ctx, catalog, in)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_subagent",
		Description: "执行指定 subagent 完成任务（一次性模式，阻塞返回文本结果）。name 来自 list_subagents，task 为具体任务描述",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runSubAgentIn) (*mcp.CallToolResult, runSubAgentOut, error) {
		return handleRunSubAgent(ctx, catalog, prefsRepo, runner, in)
	})

	return srv
}

// ====== 入参 / 出参类型 ======

type subAgentSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Model       string `json:"model,omitempty"`
}

type listSubAgentsOut struct {
	SubAgents []subAgentSummary `json:"subagents"`
}

type getSubAgentIn struct {
	Name string `json:"name" jsonschema:"subagent 的 name（来自 list_subagents）"`
}

type getSubAgentOut struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Model        string   `json:"model,omitempty"`
	AgentType    string   `json:"agent_type"`
	AllowedTools []string `json:"allowed_tools"`
}

type runSubAgentIn struct {
	Name string `json:"name" jsonschema:"要调用的 subagent 的 name"`
	Task string `json:"task" jsonschema:"具体任务描述，会作为 prompt 发送给 subagent"`
}

type runSubAgentOut struct {
	SubAgent string `json:"subagent"`
	Task     string `json:"task"`
	Result   string `json:"result"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// ====== handler 实现 ======

func handleListSubAgents(ctx context.Context, catalog SubAgentCatalog) (*mcp.CallToolResult, listSubAgentsOut, error) {
	if _, ok := userIDFrom(ctx); !ok {
		return nil, listSubAgentsOut{}, fmt.Errorf("未认证")
	}
	defs := catalog.ListSubAgents()
	out := listSubAgentsOut{SubAgents: make([]subAgentSummary, 0, len(defs))}
	for _, d := range defs {
		out.SubAgents = append(out.SubAgents, subAgentSummary{
			Name:        d.Name,
			Description: d.Description,
			Model:       d.Model,
		})
	}
	return nil, out, nil
}

func handleGetSubAgent(ctx context.Context, catalog SubAgentCatalog, in getSubAgentIn) (*mcp.CallToolResult, getSubAgentOut, error) {
	if _, ok := userIDFrom(ctx); !ok {
		return nil, getSubAgentOut{}, fmt.Errorf("未认证")
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		return nil, getSubAgentOut{}, fmt.Errorf("name 必填")
	}
	def := catalog.ResolveSubAgent(name)
	if def == nil {
		return nil, getSubAgentOut{}, fmt.Errorf("未找到 subagent: %s", name)
	}
	// AgentType 固定为"继承父 agent"：文件型 subagent 不指定后端，运行时解析。
	return nil, getSubAgentOut{
		Name:         def.Name,
		Description:  def.Description,
		Model:        def.Model,
		AgentType:    "(inherit parent agent)",
		AllowedTools: def.Tools,
	}, nil
}

func handleRunSubAgent(ctx context.Context, catalog SubAgentCatalog, prefsRepo *repository.UserAgentPrefsRepository, runner SubAgentRunner, in runSubAgentIn) (*mcp.CallToolResult, runSubAgentOut, error) {
	uid, ok := userIDFrom(ctx)
	if !ok {
		return nil, runSubAgentOut{}, fmt.Errorf("未认证")
	}
	name := strings.TrimSpace(in.Name)
	task := strings.TrimSpace(in.Task)
	if name == "" {
		return nil, runSubAgentOut{}, fmt.Errorf("name 必填")
	}
	if task == "" {
		return nil, runSubAgentOut{}, fmt.Errorf("task 必填")
	}
	def := catalog.ResolveSubAgent(name)
	if def == nil {
		return nil, runSubAgentOut{}, fmt.Errorf("未找到 subagent: %s", name)
	}

	// 文件型 subagent 不指定 agent_type，统一走"继承父 agent"：解析用户最近使用的 agent。
	agentType, err := resolveInheritedAgentType(prefsRepo, uid)
	if err != nil {
		return nil, runSubAgentOut{}, err
	}

	result, runErr := runner.RunSubAgent(ctx, acp.SubAgentRunConfig{
		AgentType:    agentType,
		ModelValue:   def.Model,
		Prompt:       task,
		SystemPrompt: def.SystemPrompt,
		UserID:       uid,
	})

	out := runSubAgentOut{SubAgent: name, Task: task}
	if runErr != nil {
		out.Success = false
		out.Error = runErr.Error()
	} else {
		out.Success = true
		out.Result = result
	}
	return nil, out, nil
}

// resolveInheritedAgentType 返回用户最近使用的 agent 类型（"继承父 agent"语义）。
// 用户从未使用过任何 agent 时返回错误。
func resolveInheritedAgentType(prefsRepo *repository.UserAgentPrefsRepository, uid uint) (string, error) {
	if prefsRepo == nil {
		return "", fmt.Errorf("无法解析继承的 agent 类型：偏好仓库未配置")
	}
	prefs, err := prefsRepo.FindByUserID(uid)
	if err != nil {
		return "", fmt.Errorf("读取用户 agent 偏好失败: %w", err)
	}
	last := strings.TrimSpace(prefs.LastAgentType)
	if last == "" {
		return "", fmt.Errorf("subagent 配置为继承父 agent，但用户尚未使用过任何 agent")
	}
	return last, nil
}
