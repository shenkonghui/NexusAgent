// Package subagentmcp 提供 opennexus-subagent MCP server，让主 agent 通过 MCP 工具调起预定义的 subagent。
//
// subagent 定义来自 markdown 文件（~/.agents/agents/*.md 等，由 acp.ScanSubAgents 扫描），
// 不再依赖数据库。frontmatter 含 name/description/model/tools，markdown 正文作为注入会话的 system_prompt。
//
// 暴露的工具：
//   - list_subagents：列出所有 subagent 摘要
//   - get_subagent：  查询单个 subagent 详情
//   - run_subagent：  执行 subagent 任务（一次性模式，阻塞返回文本结果）
//   - create_session：创建持久会话（可关联父会话）
//   - run_session_task：创建持久会话并阻塞运行一次性任务
//
// 任务编排（tasks.json）相关工具已抽离到独立的 opennexus-orchestration MCP server。
//
// 鉴权复用 opennexus-notes 的 Bearer token 体系（用户级共享一个 token），用于解析"继承父 agent"。
package subagentmcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"opennexus/internal/acp"
	"opennexus/internal/models"
	"opennexus/internal/repository"
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

// SessionCreator 由 *agent.Router 实现，用于创建持久会话（可关联父会话）。
type SessionCreator interface {
	CreateSessionWithParent(ctx context.Context, agentType string, workspaceID uint, userID uint, source, modelValue string, parentSessionID *uint) (*models.Session, error)
}

// SessionTaskRunner 由 *agent.Router 实现，用于创建持久会话并阻塞运行一次性任务。
type SessionTaskRunner interface {
	RunSessionTask(ctx context.Context, cfg acp.SessionTaskConfig) (acp.SessionTaskResult, error)
}

// SessionLookup 由 *agent.Router 实现，用于校验父会话归属（按数据库主键查询）。
type SessionLookup interface {
	GetSessionByDBID(id uint) (*models.Session, error)
}

// Handler 返回带 Bearer 鉴权的 subagent MCP Streamable HTTP Handler。
//
// prefsRepo 用于解析"继承父 agent"：subagent 文件不指定 agent 后端，运行时取用户最近使用的 agent 类型。
// sessionCreator / sessionTaskRunner / sessionLookup 用于 create_session / run_session_task 工具（可传 nil 禁用）。
func Handler(catalog SubAgentCatalog, settings *repository.NoteSettingsRepository, prefsRepo *repository.UserAgentPrefsRepository, runner SubAgentRunner, sessionCreator SessionCreator, sessionTaskRunner SessionTaskRunner, sessionLookup SessionLookup) http.Handler {
	inner := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return newServer(catalog, prefsRepo, runner, sessionCreator, sessionTaskRunner, sessionLookup)
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

func newServer(catalog SubAgentCatalog, prefsRepo *repository.UserAgentPrefsRepository, runner SubAgentRunner, sessionCreator SessionCreator, sessionTaskRunner SessionTaskRunner, sessionLookup SessionLookup) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "opennexus-subagent", Version: "1.0.0"}, nil)

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

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_session",
		Description: "创建一个新的持久会话（可作为独立会话，也可作为指定父会话的子会话/子任务）。仅创建不发送 prompt，返回 session_id 供后续对话使用。agent_type 为空时继承用户最近使用的 agent 类型。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in createSessionIn) (*mcp.CallToolResult, createSessionOut, error) {
		return handleCreateSession(ctx, prefsRepo, sessionCreator, sessionLookup, in)
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_session_task",
		Description: "创建一个新的持久会话并阻塞运行一次性任务，收集 assistant 文本后返回。区别于 run_subagent：本工具创建的会话会持久化（可通过 UI 继续对话）。agent_type 为空时继承用户最近使用的 agent 类型。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in runSessionTaskIn) (*mcp.CallToolResult, runSessionTaskOut, error) {
		return handleRunSessionTask(ctx, prefsRepo, sessionTaskRunner, sessionLookup, in)
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

// ====== create_session / run_session_task 工具 ======

// resolveAgentType 解析 agent 类型：显式指定优先，否则"继承父 agent"。
func resolveAgentType(prefsRepo *repository.UserAgentPrefsRepository, uid uint, explicit string) (string, error) {
	if at := strings.TrimSpace(explicit); at != "" {
		return at, nil
	}
	return resolveInheritedAgentType(prefsRepo, uid)
}

// validateParentOwnership 校验 parent_session_id 归属当前用户。
// 返回非 nil 的 *uint 表示合法的父会话主键；nil 表示无父级（独立会话）。
func validateParentOwnership(sessionLookup SessionLookup, uid uint, parentDBID uint) (*uint, error) {
	if parentDBID == 0 {
		return nil, nil
	}
	if sessionLookup == nil {
		return nil, fmt.Errorf("无法校验父会话归属：会话查询未配置")
	}
	parent, err := sessionLookup.GetSessionByDBID(parentDBID)
	if err != nil || parent == nil {
		return nil, fmt.Errorf("父会话不存在: %d", parentDBID)
	}
	if parent.UserID != uid {
		// 不泄露存在性，统一报不存在
		return nil, fmt.Errorf("父会话不存在: %d", parentDBID)
	}
	return &parent.ID, nil
}

// ---- create_session ----

type createSessionIn struct {
	AgentType      string `json:"agent_type,omitempty" jsonschema:"agent 类型，留空则继承用户最近使用的 agent"`
	WorkspaceID    uint   `json:"workspace_id,omitempty" jsonschema:"工作区 ID，留空则使用默认工作区"`
	ModelValue     string `json:"model_value,omitempty" jsonschema:"模型值，留空则用 agent 默认"`
	ParentSessionID uint  `json:"parent_session_id,omitempty" jsonschema:"父会话的数据库主键 ID，指定则创建子会话；留空则创建独立会话"`
}

type createSessionOut struct {
	SessionID       string `json:"session_id"`
	Title           string `json:"title,omitempty"`
	ParentSessionID uint   `json:"parent_session_id,omitempty"`
}

func handleCreateSession(ctx context.Context, prefsRepo *repository.UserAgentPrefsRepository, creator SessionCreator, sessionLookup SessionLookup, in createSessionIn) (*mcp.CallToolResult, createSessionOut, error) {
	uid, ok := userIDFrom(ctx)
	if !ok {
		return nil, createSessionOut{}, fmt.Errorf("未认证")
	}
	if creator == nil {
		return nil, createSessionOut{}, fmt.Errorf("会话创建未配置")
	}

	agentType, err := resolveAgentType(prefsRepo, uid, in.AgentType)
	if err != nil {
		return nil, createSessionOut{}, err
	}

	parentID, err := validateParentOwnership(sessionLookup, uid, in.ParentSessionID)
	if err != nil {
		return nil, createSessionOut{}, err
	}

	session, err := creator.CreateSessionWithParent(ctx, agentType, in.WorkspaceID, uid, models.SessionSourceManual, in.ModelValue, parentID)
	if err != nil {
		return nil, createSessionOut{}, err
	}

	out := createSessionOut{SessionID: session.SessionID, Title: session.Title}
	if parentID != nil {
		out.ParentSessionID = *parentID
	}
	return nil, out, nil
}

// ---- run_session_task ----

type runSessionTaskIn struct {
	AgentType       string `json:"agent_type,omitempty" jsonschema:"agent 类型，留空则继承用户最近使用的 agent"`
	Task            string `json:"task" jsonschema:"具体任务描述，会作为首条 prompt 发送给新会话"`
	WorkspaceID     uint   `json:"workspace_id,omitempty" jsonschema:"工作区 ID，留空则使用默认工作区"`
	ModelValue      string `json:"model_value,omitempty" jsonschema:"模型值，留空则用 agent 默认"`
	ParentSessionID uint   `json:"parent_session_id,omitempty" jsonschema:"父会话的数据库主键 ID，指定则创建子会话；留空则创建独立会话"`
	TimeoutSeconds  int    `json:"timeout_seconds,omitempty" jsonschema:"运行超时秒数，留空则默认 300 秒"`
}

type runSessionTaskOut struct {
	SessionID string `json:"session_id"`
	Task      string `json:"task"`
	Result    string `json:"result,omitempty"`
	Success   bool   `json:"success"`
	Error     string `json:"error,omitempty"`
}

func handleRunSessionTask(ctx context.Context, prefsRepo *repository.UserAgentPrefsRepository, runner SessionTaskRunner, sessionLookup SessionLookup, in runSessionTaskIn) (*mcp.CallToolResult, runSessionTaskOut, error) {
	uid, ok := userIDFrom(ctx)
	if !ok {
		return nil, runSessionTaskOut{}, fmt.Errorf("未认证")
	}
	if runner == nil {
		return nil, runSessionTaskOut{}, fmt.Errorf("会话任务运行未配置")
	}
	task := strings.TrimSpace(in.Task)
	if task == "" {
		return nil, runSessionTaskOut{}, fmt.Errorf("task 必填")
	}

	agentType, err := resolveAgentType(prefsRepo, uid, in.AgentType)
	if err != nil {
		return nil, runSessionTaskOut{}, err
	}

	parentID, err := validateParentOwnership(sessionLookup, uid, in.ParentSessionID)
	if err != nil {
		return nil, runSessionTaskOut{}, err
	}

	cfg := acp.SessionTaskConfig{
		AgentType:       agentType,
		ModelValue:      in.ModelValue,
		Prompt:          task,
		UserID:          uid,
		WorkspaceID:     in.WorkspaceID,
		ParentSessionID: parentID,
	}
	if in.TimeoutSeconds > 0 {
		cfg.Timeout = time.Duration(in.TimeoutSeconds) * time.Second
	}

	res, runErr := runner.RunSessionTask(ctx, cfg)

	out := runSessionTaskOut{SessionID: res.SessionID, Task: task}
	if runErr != nil {
		out.Success = false
		out.Error = runErr.Error()
	} else {
		out.Success = res.Success
		out.Result = res.Result
		out.Error = res.Error
	}
	return nil, out, nil
}

