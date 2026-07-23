package models

import "time"

// 任务编排（Orchestration）状态机常量。
const (
	OrchTaskStatusPending   = "pending"   // 已定义，尚未加入队列
	OrchTaskStatusQueued    = "queued"    // 已入队，等待执行槽位
	OrchTaskStatusRunning   = "running"   // 正在执行（agent 产出消息中）
	OrchTaskStatusDone      = "done"      // 正常完成
	OrchTaskStatusFailed    = "failed"    // 执行失败
	OrchTaskStatusCanceled  = "canceled"  // 用户手动停止
	OrchTaskStatusInterrupt = "interrupt" // 服务重启时标记的中断态
)

// IsOrchTaskRunning 报告该状态是否属于"占用执行资源"的活跃态。
func IsOrchTaskRunning(status string) bool {
	switch status {
	case OrchTaskStatusQueued, OrchTaskStatusRunning:
		return true
	default:
		return false
	}
}

// NormalizeOrchTaskStatus 将任务状态归一化为合法枚举值。
// 兼容 AI 手写 tasks.json 时常见的别名（如 completed/succeeded→done，cancelled→canceled）；
// 空值视为 pending；无法识别的值原样返回（前端会回退显示原值）。
func NormalizeOrchTaskStatus(status string) string {
	switch status {
	case "":
		return OrchTaskStatusPending
	case "completed", "succeeded", "success":
		return OrchTaskStatusDone
	case "cancelled":
		return OrchTaskStatusCanceled
	default:
		return status
	}
}

// OrchestrationTask 描述单个编排任务的定义与运行时状态。
// 持久化于工作区 cwd 下的 tasks.json（见 OrchestrationDef）。
type OrchestrationTask struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	// Detail 即任务详情，作为 prompt 发送给 agent。
	Detail     string `json:"detail"`
	AgentType  string `json:"agent_type"`
	ModelValue string `json:"model_value,omitempty"`

	// 运行时字段——执行后写回 tasks.json
	SessionID    string     `json:"session_id,omitempty"`    // 落库的稳定 session UUID
	DBSessionID  *uint      `json:"db_session_id,omitempty"` // 关联 DB Session 主键
	Status       string     `json:"status"`                  // 见 OrchTaskStatus* 常量
	Branch       string     `json:"branch,omitempty"`        // worktree 对应分支
	WorktreePath string     `json:"worktree_path,omitempty"` // worktree 绝对路径
	StartedAt    *time.Time `json:"started_at,omitempty"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
	Error        string     `json:"error,omitempty"`

	// 可扩展：任务间依赖（v1 仅做数据层，引擎按并发上限调度）。
	DependsOn []string `json:"depends_on,omitempty"`
}

// OrchestrationDef 是 tasks.json 的顶层结构。
type OrchestrationDef struct {
	MaxParallel int                 `json:"max_parallel"` // 并发上限，<=0 视为串行(=1)
	Tasks       []OrchestrationTask `json:"tasks"`
	// ParentSessionID 是编排管理会话的 DB 主键。编排任务执行时创建的会话作为其子会话
	// （通过 Session.ParentSessionID 关联），形成上下级关系。由前端在创建编排管理会话后登记。
	ParentSessionID *uint `json:"parent_session_id,omitempty"`
}

// DefaultMaxParallel 是新建编排时的默认并发上限。
const DefaultMaxParallel = 3
