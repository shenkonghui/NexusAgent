package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/robfig/cron/v3"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

// SchedulerManager 是 handler 操作定时任务所需的能力（*services.SchedulerService 实现该接口）。
type SchedulerManager interface {
	AddTask(t *models.ScheduledTask) error
	UpdateTask(t *models.ScheduledTask) error
	RemoveTask(taskID uint) error
	RunTask(taskID uint) error
}

// ExecutionLister 按会话 ID 查询定时执行块聚合（*agent.Router 实现该接口）。
type ExecutionLister interface {
	ListExecutions(sessionID string) ([]repository.ExecutionAggregate, error)
}

// ScheduledTaskWorkspaceStore 解析定时任务关联的工作区。
type ScheduledTaskWorkspaceStore interface {
	FindWorkspaceByID(id uint) (*models.Workspace, error)
}

// ScheduledTaskHandler 处理定时任务相关请求。
type ScheduledTaskHandler struct {
	repo     *repository.ScheduledTaskRepository
	execRepo *repository.TaskExecutionRepository
	mgr      SchedulerManager
	lister   ExecutionLister
	wsStore  ScheduledTaskWorkspaceStore
}

// NewScheduledTaskHandler 创建 ScheduledTaskHandler。
func NewScheduledTaskHandler(repo *repository.ScheduledTaskRepository, execRepo *repository.TaskExecutionRepository, mgr SchedulerManager, lister ExecutionLister, wsStore ScheduledTaskWorkspaceStore) *ScheduledTaskHandler {
	return &ScheduledTaskHandler{repo: repo, execRepo: execRepo, mgr: mgr, lister: lister, wsStore: wsStore}
}

type createTaskRequest struct {
	Name           string `json:"name" binding:"required"`
	AgentType      string `json:"agent_type" binding:"required"`
	WorkspaceID    uint   `json:"workspace_id"`
	Cwd            string `json:"cwd"`
	Prompt         string `json:"prompt" binding:"required"`
	CronExpr       string `json:"cron_expr" binding:"required"`
	Enabled        *bool  `json:"enabled"`
	ModelValue     string `json:"model_value"`
	TimeoutMinutes *int   `json:"timeout_minutes"`
}

// Create POST /api/v1/scheduled-tasks
func (h *ScheduledTaskHandler) Create(c *gin.Context) {
	var req createTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	if err := validateCron(req.CronExpr); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_CRON", err.Error())
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	wsID, cwd, err := h.resolveWorkspace(uid, req.WorkspaceID, req.Cwd)
	if err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	task := &models.ScheduledTask{
		Name:        strings.TrimSpace(req.Name),
		AgentType:   req.AgentType,
		WorkspaceID: wsID,
		Cwd:         cwd,
		Prompt:      req.Prompt,
		CronExpr:   req.CronExpr,
		Enabled:    enabled,
		UserID:     uid,
		ModelValue: strings.TrimSpace(req.ModelValue),
	}
	if req.TimeoutMinutes != nil && *req.TimeoutMinutes > 0 {
		task.TimeoutMinutes = *req.TimeoutMinutes
	} else {
		task.TimeoutMinutes = 5
	}
	if err := h.mgr.AddTask(task); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusCreated, task)
}

// List GET /api/v1/scheduled-tasks?workspace_id=123
// 支持按 workspace_id 过滤，不传则返回当前用户全部任务。
func (h *ScheduledTaskHandler) List(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	wsIDStr := c.Query("workspace_id")
	var tasks []models.ScheduledTask
	var err error
	if wsIDStr != "" {
		wsID, parseErr := strconv.ParseUint(wsIDStr, 10, 64)
		if parseErr != nil {
			Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "workspace_id 参数无效")
			return
		}
		tasks, err = h.repo.FindByUserIDAndWorkspace(uid, uint(wsID))
	} else {
		tasks, err = h.repo.FindByUserID(uid)
	}
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusOK, gin.H{"tasks": tasks})
}

// Get GET /api/v1/scheduled-tasks/:id
func (h *ScheduledTaskHandler) Get(c *gin.Context) {
	task, ok := h.loadOwnedTask(c)
	if !ok {
		return
	}
	Success(c, http.StatusOK, task)
}

type updateTaskRequest struct {
	Name           *string `json:"name"`
	AgentType      *string `json:"agent_type"`
	WorkspaceID    *uint   `json:"workspace_id"`
	Cwd            *string `json:"cwd"`
	Prompt         *string `json:"prompt"`
	CronExpr       *string `json:"cron_expr"`
	Enabled        *bool   `json:"enabled"`
	ModelValue     *string `json:"model_value"`
	TimeoutMinutes *int    `json:"timeout_minutes"`
}

// Update PUT /api/v1/scheduled-tasks/:id
func (h *ScheduledTaskHandler) Update(c *gin.Context) {
	task, ok := h.loadOwnedTask(c)
	if !ok {
		return
	}
	var req updateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	if req.Name != nil {
		task.Name = strings.TrimSpace(*req.Name)
	}
	if req.AgentType != nil {
		task.AgentType = *req.AgentType
	}
	if req.WorkspaceID != nil {
		wsID, cwd, err := h.resolveWorkspace(task.UserID, *req.WorkspaceID, "")
		if err != nil {
			Fail(c, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
			return
		}
		task.WorkspaceID = wsID
		task.Cwd = cwd
	} else if req.Cwd != nil {
		task.Cwd = *req.Cwd
	}
	if req.Prompt != nil {
		task.Prompt = *req.Prompt
	}
	if req.CronExpr != nil {
		if err := validateCron(*req.CronExpr); err != nil {
			Fail(c, http.StatusBadRequest, "INVALID_CRON", err.Error())
			return
		}
		task.CronExpr = *req.CronExpr
	}
	if req.Enabled != nil {
		task.Enabled = *req.Enabled
	}
	if req.ModelValue != nil {
		task.ModelValue = strings.TrimSpace(*req.ModelValue)
	}
	if req.TimeoutMinutes != nil && *req.TimeoutMinutes > 0 {
		task.TimeoutMinutes = *req.TimeoutMinutes
	}
	if err := h.mgr.UpdateTask(task); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusOK, task)
}

// Delete DELETE /api/v1/scheduled-tasks/:id
func (h *ScheduledTaskHandler) Delete(c *gin.Context) {
	task, ok := h.loadOwnedTask(c)
	if !ok {
		return
	}
	if err := h.mgr.RemoveTask(task.ID); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

// Run POST /api/v1/scheduled-tasks/:id/run — 手动触发一次执行。
func (h *ScheduledTaskHandler) Run(c *gin.Context) {
	task, ok := h.loadOwnedTask(c)
	if !ok {
		return
	}
	if err := h.mgr.RunTask(task.ID); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

// Executions GET /api/v1/scheduled-tasks/:id/executions — 任务关联会话的执行块历史（含每次执行状态）。
func (h *ScheduledTaskHandler) Executions(c *gin.Context) {
	task, ok := h.loadOwnedTask(c)
	if !ok {
		return
	}
	if task.SessionID == "" {
		Success(c, http.StatusOK, gin.H{"executions": []repository.ExecutionAggregate{}})
		return
	}
	execs, err := h.lister.ListExecutions(task.SessionID)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	// 合并 TaskExecution 表的状态
	if h.execRepo != nil && len(execs) > 0 {
		execIDs := make([]uint, 0, len(execs))
		for _, e := range execs {
			execIDs = append(execIDs, e.ExecutionID)
		}
		records, _ := h.execRepo.ListByTaskIDAndExecutionIDs(task.ID, execIDs)
		statusMap := make(map[uint]*models.TaskExecution, len(records))
		for i := range records {
			statusMap[records[i].ExecutionID] = &records[i]
		}
		for i := range execs {
			if rec, ok := statusMap[execs[i].ExecutionID]; ok {
				execs[i].Status = rec.Status
				execs[i].Error = rec.Error
			}
		}
	}
	Success(c, http.StatusOK, gin.H{"executions": execs})
}

// loadOwnedTask 加载 :id 对应任务并校验归属；失败时已写入错误响应。
func (h *ScheduledTaskHandler) loadOwnedTask(c *gin.Context) (*models.ScheduledTask, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "无效的任务 ID")
		return nil, false
	}
	task, err := h.repo.FindByID(uint(id))
	if err != nil || task == nil {
		Fail(c, http.StatusNotFound, "TASK_NOT_FOUND", "定时任务不存在")
		return nil, false
	}
	uid, ok := currentUserID(c)
	if !ok || task.UserID != uid {
		Fail(c, http.StatusNotFound, "TASK_NOT_FOUND", "定时任务不存在")
		return nil, false
	}
	return task, true
}

// resolveWorkspace 校验工作区归属并返回 workspace_id 与 cwd。
func (h *ScheduledTaskHandler) resolveWorkspace(uid, workspaceID uint, fallbackCwd string) (uint, string, error) {
	if workspaceID > 0 {
		if h.wsStore == nil {
			return 0, "", errors.New("工作区服务未配置")
		}
		ws, err := h.wsStore.FindWorkspaceByID(workspaceID)
		if err != nil || ws == nil || ws.UserID != uid {
			return 0, "", errors.New("工作区不存在")
		}
		return ws.ID, ws.Cwd, nil
	}
	if strings.TrimSpace(fallbackCwd) != "" {
		return 0, strings.TrimSpace(fallbackCwd), nil
	}
	return 0, "", errors.New("请选择工作区")
}

// validateCron 校验标准 5 字段 cron 表达式。
func validateCron(expr string) error {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	if _, err := parser.Parse(expr); err != nil {
		return errors.New("cron 表达式无效: " + err.Error())
	}
	return nil
}
