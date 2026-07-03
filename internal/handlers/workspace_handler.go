package handlers

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/models"
)

// WorkspaceStore 暴露 workspace 相关能力。
type WorkspaceStore interface {
	CreateWorkspace(ws *models.Workspace) error
	FindWorkspaceByID(id uint) (*models.Workspace, error)
	FindWorkspacesByUserID(userID uint) ([]models.Workspace, error)
	FindWorkspaceByUserIDAndCwd(userID uint, cwd string) (*models.Workspace, error)
	FindDefaultWorkspaceByUserID(userID uint) (*models.Workspace, error)
	UpdateWorkspace(id uint, updates map[string]interface{}) error
	DeleteWorkspace(id uint) error
	WorkspaceSessionCount(workspaceID uint) (int64, error)
	FindSessionsByWorkspaceID(workspaceID uint) ([]models.Session, error)
	DeleteSessionWithMessages(session *models.Session) error
	GetWorkspaceCwd(workspaceID uint) (string, error)
}

// WorkspaceHandler 处理 workspace 相关请求。
type WorkspaceHandler struct {
	store WorkspaceStore
}

// NewWorkspaceHandler 创建 WorkspaceHandler。
func NewWorkspaceHandler(store WorkspaceStore) *WorkspaceHandler {
	return &WorkspaceHandler{store: store}
}

func parseWorkspaceID(c *gin.Context) (uint, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "无效的工作区 ID")
		return 0, false
	}
	return uint(id), true
}

// Create POST /api/v1/workspaces
func (h *WorkspaceHandler) Create(c *gin.Context) {
	var req struct {
		Name        string   `json:"name" binding:"required"`
		Cwd         string   `json:"cwd" binding:"required"`
		Directories []string `json:"directories"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Cwd = strings.TrimSpace(req.Cwd)
	if req.Name == "" || req.Cwd == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "名称和目录不能为空")
		return
	}
	info, err := os.Stat(req.Cwd)
	if err != nil || !info.IsDir() {
		Fail(c, http.StatusBadRequest, "CWD_NOT_FOUND", "目录不存在: "+req.Cwd)
		return
	}
	// 校验附加目录均存在
	dirs, dirErr := validateDirectories(req.Directories)
	if dirErr != nil {
		Fail(c, http.StatusBadRequest, "DIR_NOT_FOUND", dirErr.Error())
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	if existing, _ := h.store.FindWorkspaceByUserIDAndCwd(uid, req.Cwd); existing != nil {
		Fail(c, http.StatusConflict, "DUPLICATE", "该目录已绑定工作区")
		return
	}
	ws := &models.Workspace{
		UserID:      uid,
		Name:        req.Name,
		Cwd:         req.Cwd,
		Directories: dirs,
		Mode:        models.WorkspaceModePersistent,
	}
	if err := h.store.CreateWorkspace(ws); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusCreated, ws)
}

// List GET /api/v1/workspaces
func (h *WorkspaceHandler) List(c *gin.Context) {
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	workspaces, err := h.store.FindWorkspacesByUserID(uid)
	if err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	if workspaces == nil {
		workspaces = []models.Workspace{}
	}
	type wsWithCount struct {
		models.Workspace
		SessionCount int64 `json:"session_count"`
	}
	result := make([]wsWithCount, 0, len(workspaces))
	for _, ws := range workspaces {
		count, _ := h.store.WorkspaceSessionCount(ws.ID)
		result = append(result, wsWithCount{Workspace: ws, SessionCount: count})
	}
	Success(c, http.StatusOK, gin.H{"workspaces": result})
}

// Get GET /api/v1/workspaces/:id
func (h *WorkspaceHandler) Get(c *gin.Context) {
	id, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	ws, err := h.store.FindWorkspaceByID(id)
	if err != nil || ws == nil {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	uid, _ := currentUserID(c)
	if ws.UserID != uid {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	sessions, err := h.store.FindSessionsByWorkspaceID(ws.ID)
	if err != nil {
		sessions = []models.Session{}
	}
	Success(c, http.StatusOK, gin.H{"workspace": ws, "sessions": sessions})
}

// Update PUT /api/v1/workspaces/:id
func (h *WorkspaceHandler) Update(c *gin.Context) {
	id, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	ws, err := h.store.FindWorkspaceByID(id)
	if err != nil || ws == nil {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	uid, _ := currentUserID(c)
	if ws.UserID != uid {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	var req struct {
		Name        string   `json:"name" binding:"required"`
		Directories []string `json:"directories"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "名称不能为空")
		return
	}
	updates := map[string]interface{}{"name": req.Name}
	var validatedDirs models.StringArray
	// directories 字段存在时（包括空数组）才更新
	if req.Directories != nil {
		dirs, dirErr := validateDirectories(req.Directories)
		if dirErr != nil {
			Fail(c, http.StatusBadRequest, "DIR_NOT_FOUND", dirErr.Error())
			return
		}
		validatedDirs = models.StringArray(dirs)
		updates["directories"] = validatedDirs
	}
	if err := h.store.UpdateWorkspace(id, updates); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	ws.Name = req.Name
	if req.Directories != nil {
		ws.Directories = validatedDirs
	}
	Success(c, http.StatusOK, ws)
}

// Delete DELETE /api/v1/workspaces/:id
func (h *WorkspaceHandler) Delete(c *gin.Context) {
	id, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	ws, err := h.store.FindWorkspaceByID(id)
	if err != nil || ws == nil {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	uid, _ := currentUserID(c)
	if ws.UserID != uid {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	sessions, _ := h.store.FindSessionsByWorkspaceID(ws.ID)
	for _, sess := range sessions {
		_ = h.store.DeleteSessionWithMessages(&sess)
	}
	if err := h.store.DeleteWorkspace(id); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

// Save POST /api/v1/workspaces/:id/save
func (h *WorkspaceHandler) Save(c *gin.Context) {
	id, ok := parseWorkspaceID(c)
	if !ok {
		return
	}
	ws, err := h.store.FindWorkspaceByID(id)
	if err != nil || ws == nil {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	uid, _ := currentUserID(c)
	if ws.UserID != uid {
		Fail(c, http.StatusNotFound, "NOT_FOUND", "工作区不存在")
		return
	}
	if ws.Mode != models.WorkspaceModeTemporary {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "只能保存临时工作区")
		return
	}
	var req struct {
		Name        string   `json:"name" binding:"required"`
		Cwd         string   `json:"cwd" binding:"required"`
		Directories []string `json:"directories"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Cwd = strings.TrimSpace(req.Cwd)
	if req.Name == "" || req.Cwd == "" {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "名称和目录不能为空")
		return
	}
	info, err := os.Stat(req.Cwd)
	if err != nil || !info.IsDir() {
		Fail(c, http.StatusBadRequest, "CWD_NOT_FOUND", "目录不存在: "+req.Cwd)
		return
	}
	dirs, dirErr := validateDirectories(req.Directories)
	if dirErr != nil {
		Fail(c, http.StatusBadRequest, "DIR_NOT_FOUND", dirErr.Error())
		return
	}
	if err := h.store.UpdateWorkspace(id, map[string]interface{}{
		"name":        req.Name,
		"cwd":         req.Cwd,
		"directories": models.StringArray(dirs),
		"mode":        models.WorkspaceModePersistent,
	}); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	ws.Name = req.Name
	ws.Cwd = req.Cwd
	ws.Mode = models.WorkspaceModePersistent
	ws.Directories = dirs
	Success(c, http.StatusOK, ws)
}

// validateDirectories 去重并校验每个附加目录存在且是目录。
// 返回去重后的路径切片。
func validateDirectories(dirs []string) ([]string, error) {
	seen := make(map[string]bool)
	result := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		info, err := os.Stat(d)
		if err != nil || !info.IsDir() {
			return nil, fmt.Errorf("附加目录不存在: %s", d)
		}
		result = append(result, d)
	}
	return result, nil
}
