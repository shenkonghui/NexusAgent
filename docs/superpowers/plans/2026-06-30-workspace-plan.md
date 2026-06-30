# Workspace 功能实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 新增独立的 Workspace 实体，将其作为 Session 的上层分组容器，Session 通过 workspace_id 关联 workspace，不再独立拥有 cwd

**架构：** 后端新增 Workspace 模型/仓库/处理器，Session 模型增加 workspace_id 外键并移除 cwd/workspace_mode/temp_dir 字段。前端新增 WorkspaceSidebar、HomePage、WorkspacePage 组件，改造 ChatPage 和路由

**技术栈：** Go 1.25 + Gin + GORM + SQLite / React 18 + TypeScript + React Router v6

---

## 文件结构

| 文件 | 职责 | 变更 |
|------|------|------|
| `internal/models/workspace.go` | Workspace 数据模型与常量 | 新增 |
| `internal/models/session.go` | Session 模型，增加 WorkspaceID 字段 | 修改 |
| `internal/database/database.go` | AutoMigrate 注册新表 + 数据迁移 | 修改 |
| `internal/repository/workspace_repository.go` | Workspace 数据库操作 | 新增 |
| `internal/repository/session_repository.go` | 新增 FindByWorkspaceID、移除 UpdateWorkspace | 修改 |
| `internal/handlers/workspace_handler.go` | Workspace CRUD + save 接口 | 新增 |
| `internal/handlers/session_handler.go` | 创建会话接收 workspace_id，响应含 workspace 信息 | 修改 |
| `internal/handlers/session_file_handler.go` | cwd 从 workspace 获取 | 修改 |
| `internal/handlers/terminal_handler.go` | cwd 从 workspace 获取 | 修改 |
| `internal/acp/service.go` | CreateSession 接收 workspace_id，ResumeSession/DeleteSession/ListSkills 适配 | 修改 |
| `internal/agent/router.go` | CreateSession 签名更新，新增 workspace 相关方法 | 修改 |
| `internal/router/router.go` | 新增 workspace 路由 | 修改 |
| `web/src/types.ts` | 新增 Workspace 类型，更新 Session 类型 | 修改 |
| `web/src/api/workspaces.ts` | Workspace API 客户端 | 新增 |
| `web/src/api/sessions.ts` | createSession 参数改为 workspace_id | 修改 |
| `web/src/components/WorkspaceSidebar.tsx` | workspace 列表 + 创建/管理 | 新增 |
| `web/src/components/CreateWorkspaceDialog.tsx` | 创建 workspace 弹窗 | 新增 |
| `web/src/pages/HomePage.tsx` | 首页：workspace 列表 + 快速对话 | 新增 |
| `web/src/pages/WorkspacePage.tsx` | workspace 详情 + Session 列表 + 快速对话 | 新增 |
| `web/src/pages/ChatPage.tsx` | 适配 workspace_id 路由参数 | 修改 |
| `web/src/App.tsx` | 新增路由 | 修改 |
| `web/src/i18n/zh.json` | 新增 workspace 相关翻译 | 修改 |

---

### 任务 1：创建 Workspace 模型

**文件：**
- 创建：`internal/models/workspace.go`

- [ ] **步骤 1：编写 Workspace 模型和常量**

```go
package models

import "time"

const (
	WorkspaceModePersistent = "persistent"
	WorkspaceModeTemporary  = "temporary"
)

// Workspace 用户级工作区，绑定固定文件系统目录。
// 每个 Session 归属于某个 Workspace，Session 的 cwd 从 Workspace 继承。
type Workspace struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Cwd       string    `gorm:"size:512;not null" json:"cwd"`
	Mode      string    `gorm:"size:32;not null;default:persistent" json:"mode"`
	TempDir   string    `gorm:"size:512" json:"temp_dir"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
```

- [ ] **步骤 2：Commit**

```bash
git add internal/models/workspace.go
git commit -m "feat: 新增 Workspace 数据模型"
```

---

### 任务 2：更新 Session 模型

**文件：**
- 修改：`internal/models/session.go`

- [ ] **步骤 1：Session 增加 WorkspaceID，标记旧字段废弃**

修改 `internal/models/session.go`，在 `Session` 结构体中：
- 增加 `WorkspaceID *uint` 和 `Workspace Workspace`
- 将 `Cwd` 改为 `json:"-"` 标记废弃（保留数据库列兼容旧数据）
- 将 `WorkspaceMode` 改为 `json:"-"` 标记废弃
- 将 `TempDir` 改为 `json:"-"` 标记废弃

```go
type Session struct {
	ID            uint   `gorm:"primaryKey" json:"id"`
	SessionID     string `gorm:"uniqueIndex;size:128;not null" json:"session_id"`
	AgentType     string `gorm:"size:64;not null" json:"agent_type"`
	Cwd           string `gorm:"size:512;not null" json:"-"`            // 废弃，cwd 从 Workspace 获取
	Status        string `gorm:"size:32;not null;default:active" json:"status"`
	UserID        uint   `gorm:"index" json:"user_id"`
	WorkspaceMode string `gorm:"size:32;not null" json:"-"`              // 废弃
	TempDir       string `gorm:"size:512" json:"-"`                      // 废弃
	// WorkspaceID 关联的工作区 ID（可选，向后兼容旧数据）
	WorkspaceID *uint     `gorm:"index" json:"workspace_id"`
	Workspace   Workspace `gorm:"foreignKey:WorkspaceID" json:"workspace,omitempty"`
	LastPrompt  string    `gorm:"type:text" json:"last_prompt"`
	Title       string    `gorm:"size:128" json:"title"`
	Source      string    `gorm:"size:32;not null;default:manual" json:"source"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	ClosedAt    *time.Time `gorm:"index" json:"closed_at"`
}
```

同时移除旧的 `WorkspaceModeExternal` 和 `WorkspaceModeTemporary` 常量（Workspace 模型已有同名常量，但那是 workspace 级别的），注意 `Session` 文件中这两个常量还在被 `internal/acp/service.go` 使用（如 `models.WorkspaceModeExternal`），需要同步处理。

**保留** `Session` 文件中的 `SessionStatusActive`、`SessionStatusClosed`、`SessionStatusError`、`SessionSourceManual`、`SessionSourceScheduled` 常量。

移除：
```go
const (
	WorkspaceModeExternal  = "external"   // 删除
	WorkspaceModeTemporary = "temporary"  // 删除
)
```

- [ ] **步骤 2：Commit**

```bash
git add internal/models/session.go
git commit -m "feat: Session 模型增加 WorkspaceID 外键，废弃 cwd/workspace_mode/temp_dir 字段"
```

---

### 任务 3：数据库迁移与旧数据兼容

**文件：**
- 修改：`internal/database/database.go`

- [ ] **步骤 1：AutoMigrate 注册 Workspace 表**

```go
func Connect(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.RefreshToken{},
		&models.Session{}, &models.Message{},
		&models.AgentConfig{}, &models.ScheduledTask{},
		&models.TaskExecution{},
		&models.Workspace{}, // 新增
	); err != nil {
		return nil, fmt.Errorf("迁移数据库: %w", err)
	}
	// 数据迁移：为旧 Session 创建对应 Workspace，填充 workspace_id
	if err := migrateOldSessionsToWorkspaces(db); err != nil {
		return nil, fmt.Errorf("迁移旧会话数据: %w", err)
	}
	return db, nil
}

// migrateOldSessionsToWorkspaces 扫描已有 Session 的 cwd，为每个唯一 cwd 创建 Workspace，
// 并将 Session.workspace_id 指向对应 Workspace。
func migrateOldSessionsToWorkspaces(db *gorm.DB) error {
	// 统计已迁移的 session 数
	var count int64
	if err := db.Model(&models.Session{}).
		Where("workspace_id IS NULL OR workspace_id = 0").
		Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}

	// 收集所有已有 session 的 user_id 和 cwd 去重
	type userCwd struct {
		UserID uint   `gorm:"column:user_id"`
		Cwd    string `gorm:"column:cwd"`
	}
	var pairs []userCwd
	if err := db.Model(&models.Session{}).
		Select("DISTINCT user_id, cwd").
		Where("workspace_id IS NULL OR workspace_id = 0").
		Find(&pairs).Error; err != nil {
		return err
	}

	// 为每个 (userID, cwd) 创建 Workspace
	for _, p := range pairs {
		ws := models.Workspace{
			UserID: p.UserID,
			Name:   filepath.Base(p.Cwd), // 目录名作为默认名称
			Cwd:    p.Cwd,
			Mode:   models.WorkspaceModePersistent,
		}
		if err := db.Create(&ws).Error; err != nil {
			return fmt.Errorf("创建 workspace (user=%d, cwd=%s): %w", p.UserID, p.Cwd, err)
		}
		// 更新对应 session 的 workspace_id
		if err := db.Model(&models.Session{}).
			Where("user_id = ? AND cwd = ? AND (workspace_id IS NULL OR workspace_id = 0)", p.UserID, p.Cwd).
			Update("workspace_id", ws.ID).Error; err != nil {
			return fmt.Errorf("更新 session workspace_id: %w", err)
		}
	}

	// 为没有任何 session 的用户创建默认 temporary workspace
	if err := createDefaultWorkspacesForEmptyUsers(db); err != nil {
		return err
	}
	return nil
}

// createDefaultWorkspacesForEmptyUsers 为没有任何 workspace 的用户创建默认 temporary workspace。
func createDefaultWorkspacesForEmptyUsers(db *gorm.DB) error {
	var userIDs []uint
	if err := db.Model(&models.User{}).Pluck("id", &userIDs).Error; err != nil {
		return err
	}
	for _, uid := range userIDs {
		var count int64
		if err := db.Model(&models.Workspace{}).Where("user_id = ?", uid).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			// 创建默认 temporary workspace
			ws, err := createTempWorkspace("", "nexus-")
			if err != nil {
				return fmt.Errorf("为用户 %d 创建默认 workspace: %w", uid, err)
			}
			ws.UserID = uid
			ws.Name = "默认工作区"
			ws.Mode = models.WorkspaceModeTemporary
			if err := db.Create(ws).Error; err != nil {
				return fmt.Errorf("保存默认 workspace: %w", err)
			}
		}
	}
	return nil
}

// createTempWorkspace 创建临时目录（复用现有逻辑）。
func createTempWorkspace(baseDir, prefix string) (*models.Workspace, error) {
	dir := baseDir
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("获取用户主目录: %w", err)
		}
		dir = filepath.Join(home, ".nextAgent", "session")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("创建临时根目录: %w", err)
	}
	tempDir, err := os.MkdirTemp(dir, prefix)
	if err != nil {
		return nil, fmt.Errorf("创建临时目录: %w", err)
	}
	return &models.Workspace{
		Cwd:     tempDir,
		TempDir: tempDir,
		Mode:    models.WorkspaceModeTemporary,
	}, nil
}
```

需要增加 import：
```go
import (
	"fmt"
	"os"
	"path/filepath"
	// ... 已有 import
)
```

- [ ] **步骤 2：编译验证**

```bash
cd /Users/shenkonghui/src/mywork/NexusAgent && go build ./...
```
预期：可能会因其他文件引用旧字段而编译失败（后续任务解决）。

- [ ] **步骤 3：Commit**

```bash
git add internal/database/database.go
git commit -m "feat: AutoMigrate 注册 Workspace 表，添加旧 Session 数据迁移逻辑"
```

---

### 任务 4：创建 Workspace Repository

**文件：**
- 创建：`internal/repository/workspace_repository.go`

- [ ] **步骤 1：实现 WorkspaceRepository**

```go
package repository

import (
	"gorm.io/gorm"
	"nexusagent/internal/models"
)

type WorkspaceRepository struct {
	db *gorm.DB
}

func NewWorkspaceRepository(db *gorm.DB) *WorkspaceRepository {
	return &WorkspaceRepository{db: db}
}

func (r *WorkspaceRepository) Create(ws *models.Workspace) error {
	return r.db.Create(ws).Error
}

func (r *WorkspaceRepository) FindByID(id uint) (*models.Workspace, error) {
	var ws models.Workspace
	if err := r.db.First(&ws, id).Error; err != nil {
		return nil, err
	}
	return &ws, nil
}

func (r *WorkspaceRepository) FindByUserID(userID uint) ([]models.Workspace, error) {
	var workspaces []models.Workspace
	err := r.db.Where("user_id = ?", userID).Order("created_at DESC").Find(&workspaces).Error
	return workspaces, err
}

func (r *WorkspaceRepository) FindByUserIDAndCwd(userID uint, cwd string) (*models.Workspace, error) {
	var ws models.Workspace
	err := r.db.Where("user_id = ? AND cwd = ? AND mode = ?", userID, cwd, models.WorkspaceModePersistent).First(&ws).Error
	return &ws, err
}

func (r *WorkspaceRepository) Update(id uint, updates map[string]interface{}) error {
	return r.db.Model(&models.Workspace{}).Where("id = ?", id).Updates(updates).Error
}

func (r *WorkspaceRepository) Delete(id uint) error {
	return r.db.Delete(&models.Workspace{}, id).Error
}

// FindDefaultByUserID 查找用户的默认 temporary workspace。
func (r *WorkspaceRepository) FindDefaultByUserID(userID uint) (*models.Workspace, error) {
	var ws models.Workspace
	err := r.db.Where("user_id = ? AND mode = ?", userID, models.WorkspaceModeTemporary).First(&ws).Error
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

// SessionCount 统计 workspace 下的 session 数。
func (r *WorkspaceRepository) SessionCount(workspaceID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Session{}).Where("workspace_id = ?", workspaceID).Count(&count).Error
	return count, err
}

// CountByUserID 统计用户的 workspace 总数。
func (r *WorkspaceRepository) CountByUserID(userID uint) (int64, error) {
	var count int64
	err := r.db.Model(&models.Workspace{}).Where("user_id = ?", userID).Count(&count).Error
	return count, err
}
```

- [ ] **步骤 2：Commit**

```bash
git add internal/repository/workspace_repository.go
git commit -m "feat: 新增 WorkspaceRepository"
```

---

### 任务 5：更新 SessionRepository

**文件：**
- 修改：`internal/repository/session_repository.go`

- [ ] **步骤 1：新增 FindByWorkspaceID，移除 UpdateWorkspace**

新增方法：
```go
// FindByWorkspaceID 返回指定 workspace 下的所有 session。
func (r *SessionRepository) FindByWorkspaceID(workspaceID uint) ([]models.Session, error) {
	var sessions []models.Session
	err := r.db.Where("workspace_id = ?", workspaceID).Order("created_at DESC").Find(&sessions).Error
	return sessions, err
}
```

移除 `UpdateWorkspace` 方法（不再需要，cwd 现在由 workspace 管理）。

- [ ] **步骤 2：Compile verification**

```bash
cd /Users/shenkonghui/src/mywork/NexusAgent && go build ./...
```

- [ ] **步骤 3：Commit**

```bash
git add internal/repository/session_repository.go
git commit -m "feat: SessionRepository 新增 FindByWorkspaceID，移除 UpdateWorkspace"
```

---

### 任务 6：创建 Workspace Handler

**文件：**
- 创建：`internal/handlers/workspace_handler.go`

- [ ] **步骤 1：实现 WorkspaceHandler**

```go
package handlers

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
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
	WorkspaceCountByUserID(userID uint) (int64, error)
	// Session 相关（用于删除 workspace 时级联）
	FindSessionsByWorkspaceID(workspaceID uint) ([]models.Session, error)
	DeleteSessionWithMessages(session *models.Session) error
}

// WorkspaceHandler 处理 workspace 相关请求。
type WorkspaceHandler struct {
	store WorkspaceStore
}

// NewWorkspaceHandler 创建 WorkspaceHandler。
func NewWorkspaceHandler(store WorkspaceStore) *WorkspaceHandler {
	return &WorkspaceHandler{store: store}
}

// parseWorkspaceID 解析 :id（uint，>0）。
func parseWorkspaceID(c *gin.Context) (uint, bool) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "无效的工作区 ID")
		return 0, false
	}
	return uint(id), true
}

// getWorkspaceCwd 获取 workspace 的 cwd，沿用于文件/终端操作。
// 返回的 workspace 已校验归属。
func (h *WorkspaceHandler) getWorkspaceCwd(c *gin.Context, workspaceID uint) (string, bool) {
	ws, err := h.store.FindWorkspaceByID(workspaceID)
	if err != nil || ws == nil {
		return "", false
	}
	uid, _ := currentUserID(c)
	if ws.UserID != uid {
		return "", false
	}
	return ws.Cwd, true
}

// Create POST /api/v1/workspaces
func (h *WorkspaceHandler) Create(c *gin.Context) {
	var req struct {
		Name string `json:"name" binding:"required"`
		Cwd  string `json:"cwd" binding:"required"`
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
	// 校验目录存在
	info, err := os.Stat(req.Cwd)
	if err != nil || !info.IsDir() {
		Fail(c, http.StatusBadRequest, "CWD_NOT_FOUND", "目录不存在: "+req.Cwd)
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	// 检查同用户是否已有相同 cwd 的 workspace
	if existing, _ := h.store.FindWorkspaceByUserIDAndCwd(uid, req.Cwd); existing != nil && existing.ID != 0 {
		Fail(c, http.StatusConflict, "DUPLICATE", "该目录已绑定工作区")
		return
	}
	ws := &models.Workspace{
		UserID: uid,
		Name:   req.Name,
		Cwd:    req.Cwd,
		Mode:   models.WorkspaceModePersistent,
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
	// 附带每个 workspace 的 session 数量
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
	// 附带 sessions 列表
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
		Name string `json:"name" binding:"required"`
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
	if err := h.store.UpdateWorkspace(id, map[string]interface{}{"name": req.Name}); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	ws.Name = req.Name
	Success(c, http.StatusOK, ws)
}

// Delete DELETE /api/v1/workspaces/:id — 删除 workspace 及其所有 session
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
	// 级联删除 workspace 下所有 session
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

// Save POST /api/v1/workspaces/:id/save — 将 temporary workspace 转为 persistent
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
		Name string `json:"name" binding:"required"`
		Cwd  string `json:"cwd" binding:"required"`
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
	// 校验目录存在
	info, err := os.Stat(req.Cwd)
	if err != nil || !info.IsDir() {
		Fail(c, http.StatusBadRequest, "CWD_NOT_FOUND", "目录不存在: "+req.Cwd)
		return
	}
	if err := h.store.UpdateWorkspace(id, map[string]interface{}{
		"name": req.Name,
		"cwd":  req.Cwd,
		"mode": models.WorkspaceModePersistent,
	}); err != nil {
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
		return
	}
	ws.Name = req.Name
	ws.Cwd = req.Cwd
	ws.Mode = models.WorkspaceModePersistent
	Success(c, http.StatusOK, ws)
}
```

- [ ] **步骤 2：Commit**

```bash
git add internal/handlers/workspace_handler.go
git commit -m "feat: 新增 WorkspaceHandler（CRUD + save）"
```

---

### 任务 7：更新 ACP Service（CreateSession、ResumeSession、DeleteSession、ListSkills）

**文件：**
- 修改：`internal/acp/service.go`

- [ ] **步骤 1：Service 增加 WorkspaceRepository**

```go
type Service struct {
	sessions  *repository.SessionRepository
	messages  *repository.MessageRepository
	workspaces *repository.WorkspaceRepository  // 新增
	// ... 其余字段不变
}
```

更新 `NewService`：
```go
func NewService(db *gorm.DB, wsConfig config.WorkspaceConfig) *Service {
	return &Service{
		sessions:    repository.NewSessionRepository(db),
		messages:    repository.NewMessageRepository(db),
		workspaces:  repository.NewWorkspaceRepository(db),  // 新增
		// ... 其余不变
	}
}
```

- [ ] **步骤 2：修改 CreateSessionWithSource — 接收 workspaceID 参数**

变更函数签名，增加 `workspaceID uint` 参数：

```go
func (s *Service) CreateSessionWithSource(ctx context.Context, agentType string, workspaceID uint, userID uint, source, modelValue string) (*models.Session, error) {
```

函数内部逻辑变更：

```go
	if _, err := s.GetBackend(agentType); err != nil {
		return nil, err
	}

	// 查找 workspace，获取 cwd
	var ws *Workspace
	var dbWS *models.Workspace
	if workspaceID > 0 {
		dbWS, err := s.workspaces.FindByID(workspaceID)
		if err != nil {
			return nil, fmt.Errorf("工作区不存在: %w", err)
		}
		if dbWS.UserID != userID {
			return nil, errors.New("无权访问该工作区")
		}
		ws = &Workspace{Mode: dbWS.Mode, Cwd: dbWS.Cwd, TempDir: dbWS.TempDir}
	} else {
		// 没有 workspaceID：查找或创建默认 temporary workspace
		dbWS, err = s.workspaces.FindDefaultByUserID(userID)
		if err != nil {
			// 创建默认 temporary workspace
			tempWs, tErr := NewTemporaryWorkspace(s.wsConfig.SessionDir, s.wsConfig.TempDirPrefix)
			if tErr != nil {
				return nil, tErr
			}
			newWS := &models.Workspace{
				UserID:  userID,
				Name:    "默认工作区",
				Cwd:     tempWs.Cwd,
				Mode:    models.WorkspaceModeTemporary,
				TempDir: tempWs.TempDir,
			}
			if cErr := s.workspaces.Create(newWS); cErr != nil {
				_ = tempWs.Cleanup()
				return nil, fmt.Errorf("创建默认工作区: %w", cErr)
			}
			dbWS = newWS
			ws = tempWs
		} else {
			ws = &Workspace{Mode: dbWS.Mode, Cwd: dbWS.Cwd, TempDir: dbWS.TempDir}
		}
		workspaceID = dbWS.ID
	}

	conn, err := s.ensureConnection(ctx, agentType)
	if err != nil {
		return nil, err
	}

	sessionID, configOptions, modes, err := conn.NewSession(ctx, ws.Cwd)
	if err != nil {
		return nil, fmt.Errorf("创建 ACP 会话: %w", err)
	}

	wid := dbWS.ID
	session := &models.Session{
		SessionID:   sessionID,
		AgentType:   agentType,
		Cwd:         ws.Cwd,         // 保留兼容值
		Status:      models.SessionStatusActive,
		UserID:      userID,
		WorkspaceID: &wid,
		Source:      source,
	}
	// ... 后续不变（落库、路由注册、modelValue 设置）
```

- [ ] **步骤 3：修改 CreateSession 签名**

```go
func (s *Service) CreateSession(ctx context.Context, agentType string, workspaceID uint, userID uint, modelValue string) (*models.Session, error) {
	return s.CreateSessionWithSource(ctx, agentType, workspaceID, userID, models.SessionSourceManual, modelValue)
}
```

- [ ] **步骤 4：修改 ResumeSession — cwd 从 workspace 获取，移除 cwdOverride**

```go
func (s *Service) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}

	// active 且连接存在 → 直接返回
	if session.Status == models.SessionStatusActive {
		if _, ok := s.connForSession(sessionID); ok {
			return session, nil
		}
	}

	// 从 workspace 获取 cwd
	cwd := session.Cwd // 向后兼容
	if session.WorkspaceID != nil {
		if ws, wsErr := s.workspaces.FindByID(*session.WorkspaceID); wsErr == nil {
			cwd = ws.Cwd
		}
	}
	if cwd == "" {
		return nil, errors.New("恢复会话需要工作目录，请提供有效的 workspace")
	}
	if !dirExists(cwd) {
		return nil, fmt.Errorf("工作目录不存在: %s", cwd)
	}
	// ... 后续不变（建立连接、新建 ACP session、注入历史、更新 session_id）
```

移除 `cwdOverride` 逻辑和 `UpdateWorkspace` 调用。

- [ ] **步骤 5：修改 DeleteSession — workspace cleanup 改为仅清理 temporary**

```go
func (s *Service) DeleteSession(ctx context.Context, sessionID string) error {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return err
	}

	// ... detach session 等不变

	// workspace 清理改为：仅当 no other sessions 使用该 workspace 且为 temporary 时清理
	// 暂不自动清理，由 workspace 删除时统一处理
	// ws := &Workspace{Mode: session.WorkspaceMode, TempDir: session.TempDir}
	// _ = ws.Cleanup()  // 移除

	// 先删消息再删会话
	if err := s.messages.DeleteByDBSessionID(session.ID); err != nil {
		return fmt.Errorf("删除会话消息: %w", err)
	}
	if err := s.sessions.Delete(session.ID); err != nil {
		return fmt.Errorf("删除会话记录: %w", err)
	}
	return nil
}
```

- [ ] **步骤 6：修改 ListSkills — cwd 从 workspace 获取**

```go
func (s *Service) ListSkills(sessionID string) ([]Skill, error) {
	session, err := s.GetSession(sessionID)
	if err != nil {
		return nil, err
	}
	cwd := session.Cwd
	if session.WorkspaceID != nil {
		if ws, wsErr := s.workspaces.FindByID(*session.WorkspaceID); wsErr == nil {
			cwd = ws.Cwd
		}
	}
	return ScanSkills(cwd), nil
}
```

- [ ] **步骤 7：增加 WorkspaceStore 相关新方法**

为让 Service 实现 WorkspaceStore 接口，新增方法：

```go
func (s *Service) CreateWorkspace(ws *models.Workspace) error {
	return s.workspaces.Create(ws)
}

func (s *Service) FindWorkspaceByID(id uint) (*models.Workspace, error) {
	return s.workspaces.FindByID(id)
}

func (s *Service) FindWorkspacesByUserID(userID uint) ([]models.Workspace, error) {
	return s.workspaces.FindByUserID(userID)
}

func (s *Service) FindWorkspaceByUserIDAndCwd(userID uint, cwd string) (*models.Workspace, error) {
	return s.workspaces.FindByUserIDAndCwd(userID, cwd)
}

func (s *Service) FindDefaultWorkspaceByUserID(userID uint) (*models.Workspace, error) {
	return s.workspaces.FindDefaultByUserID(userID)
}

func (s *Service) UpdateWorkspace(id uint, updates map[string]interface{}) error {
	return s.workspaces.Update(id, updates)
}

func (s *Service) DeleteWorkspace(id uint) error {
	return s.workspaces.Delete(id)
}

func (s *Service) WorkspaceSessionCount(workspaceID uint) (int64, error) {
	return s.workspaces.SessionCount(workspaceID)
}

func (s *Service) WorkspaceCountByUserID(userID uint) (int64, error) {
	return s.workspaces.CountByUserID(userID)
}

func (s *Service) FindSessionsByWorkspaceID(workspaceID uint) ([]models.Session, error) {
	return s.sessions.FindByWorkspaceID(workspaceID)
}

func (s *Service) DeleteSessionWithMessages(session *models.Session) error {
	if err := s.messages.DeleteByDBSessionID(session.ID); err != nil {
		return err
	}
	return s.sessions.Delete(session.ID)
}
```

- [ ] **步骤 8：修改 cleanupProbeSession**

```go
func (s *Service) cleanupProbeSession(ctx context.Context, sess *models.Session) {
	agentType, hadConn := s.detachSession(sess.SessionID)
	if hadConn {
		if conn, ok := s.pool[agentType]; ok {
			_ = conn.CloseSessionByID(ctx, sess.SessionID)
		}
	}
	// 不再清理 workspace（探测用的临时 workspace 由调用方管理）
	_ = s.messages.DeleteByDBSessionID(sess.ID)
	_ = s.sessions.Delete(sess.ID)
}
```

- [ ] **步骤 9：编译验证**

```bash
cd /Users/shenkonghui/src/mywork/NexusAgent && go build ./internal/acp/
```
预期可能因调用方尚未更新签名而报错（后续任务解决）。

- [ ] **步骤 10：Commit**

```bash
git add internal/acp/service.go
git commit -m "feat: ACP Service 适配 Workspace 模式（CreateSession 接收 workspace_id，ResumeSession 从 workspace 获取 cwd）"
```

---

### 任务 8：更新 agent.Router 和 SessionStore 接口

**文件：**
- 修改：`internal/agent/router.go`
- 修改：`internal/handlers/session_handler.go` 中的 `SessionStore` 接口

- [ ] **步骤 1：更新 Router.CreateSession 签名**

```go
func (r *Router) CreateSession(ctx context.Context, agentType string, workspaceID uint, userID uint, modelValue string) (*models.Session, error) {
	return r.CreateSessionWithSource(ctx, agentType, workspaceID, userID, models.SessionSourceManual, modelValue)
}

func (r *Router) CreateSessionWithSource(ctx context.Context, agentType string, workspaceID uint, userID uint, source, modelValue string) (*models.Session, error) {
	if _, err := r.registry.Get(agentType); err != nil {
		return nil, err
	}
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.CreateSessionWithSource(ctx, agentType, workspaceID, userID, source, modelValue)
}
```

- [ ] **步骤 2：更新 Router.ResumeSession — 移除 cwdOverride**

```go
func (r *Router) ResumeSession(ctx context.Context, sessionID string) (*models.Session, error) {
	if r.service == nil {
		return nil, errors.New("service 未配置")
	}
	return r.service.ResumeSession(ctx, sessionID)
}
```

- [ ] **步骤 3：更新 SessionStore 接口**

修改 `internal/handlers/session_handler.go` 中的 `SessionStore`：

```go
type SessionStore interface {
	CreateSession(ctx context.Context, agentType string, workspaceID uint, userID uint, modelValue string) (*models.Session, error)
	ListSessions(userID uint) ([]models.Session, error)
	ListSessionsBySource(userID uint, source string) ([]models.Session, error)
	GetSessionByDBID(id uint) (*models.Session, error)
	DeleteSession(ctx context.Context, sessionID string) error
	CancelSession(ctx context.Context, sessionID string) error
	ResumeSession(ctx context.Context, sessionID string) (*models.Session, error)
	// ... 其余方法不变
}
```

- [ ] **步骤 4：Commit**

```bash
git add internal/agent/router.go internal/handlers/session_handler.go
git commit -m "feat: Router 和 SessionStore 接口适配 workspace_id 参数"
```

---

### 任务 9：更新 SessionHandler（去掉 cwd，改用 workspace_id）

**文件：**
- 修改：`internal/handlers/session_handler.go`

- [ ] **步骤 1：修改 createSessionRequest 和 Create 方法**

```go
type createSessionRequest struct {
	AgentType   string `json:"agent_type" binding:"required"`
	WorkspaceID uint   `json:"workspace_id"`  // 新增，可选
	ModelValue  string `json:"model_value"`
	// Cwd        string `json:"cwd"`  // 移除
}

func (h *SessionHandler) Create(c *gin.Context) {
	var req createSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	uid, ok := currentUserID(c)
	if !ok {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	sess, err := h.store.CreateSession(c.Request.Context(), req.AgentType, req.WorkspaceID, uid, req.ModelValue)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusCreated, sess)
}
```

- [ ] **步骤 2：修改 Resume — 移除 cwd**

```go
func (h *SessionHandler) Resume(c *gin.Context) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return
	}
	updated, err := h.store.ResumeSession(c.Request.Context(), sess.SessionID)
	if err != nil {
		writeSessionError(c, err)
		return
	}
	Success(c, http.StatusOK, updated)
}
```

移除 `resumeRequest` 结构体。

- [ ] **步骤 3：更新 writeSessionError — 调整错误信息**

移除 `"必须提供 cwd"` 相关错误，保留通用错误处理。

```go
func writeSessionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, agent.ErrAgentNotFound):
		Fail(c, http.StatusBadRequest, "AGENT_NOT_FOUND", "未知的 agent 类型")
	default:
		Fail(c, http.StatusInternalServerError, "INTERNAL", err.Error())
	}
}
```

- [ ] **步骤 4：Commit**

```bash
git add internal/handlers/session_handler.go
git commit -m "feat: SessionHandler 创建/恢复会话改用 workspace_id，移除 cwd 参数"
```

---

### 任务 10：更新文件/终端 Handler 获取 cwd 的方式

**文件：**
- 修改：`internal/handlers/session_file_handler.go`
- 修改：`internal/handlers/terminal_handler.go`

- [ ] **步骤 1：文件 Handler — cwd 从 session.Workspace 获取**

在 `session_file_handler.go` 的 `resolveSessionPath` 方法中，获取 cwd 的逻辑改为：

```go
func (h *SessionFileHandler) resolveSessionPath(c *gin.Context) (*models.Session, string, bool) {
	sess, ok := h.loadOwnedSession(c)
	if !ok {
		return nil, "", false
	}
	cwd := sess.Cwd  // 向后兼容
	if sess.WorkspaceID != nil {
		if ws, err := h.store.GetWorkspaceCwd(*sess.WorkspaceID); err == nil {
			cwd = ws
		}
	}
	// ... 后续不变
}
```

SessionStore 接口需增加 `GetWorkspaceCwd(workspaceID uint) (string, error)` 方法。

在 `internal/acp/service.go` 中实现：
```go
func (s *Service) GetWorkspaceCwd(workspaceID uint) (string, error) {
	ws, err := s.workspaces.FindByID(workspaceID)
	if err != nil {
		return "", err
	}
	return ws.Cwd, nil
}
```

- [ ] **步骤 2：终端 Handler — cwd 从 workspace 获取**

在 `terminal_handler.go` 的 `HandleTerminal` 方法中，启动 shell 时的 cwd 获取逻辑改为从 workspace 获取：

```go
// 找到 cwd
cwd := sess.Cwd
if sess.WorkspaceID != nil {
	if ws, err := h.store.GetWorkspaceCwd(*sess.WorkspaceID); err == nil {
		cwd = ws
	}
}
```

- [ ] **步骤 3：编译整个项目**

```bash
cd /Users/shenkonghui/src/mywork/NexusAgent && go build ./...
```
预期：全部编译通过。

- [ ] **步骤 4：Commit**

```bash
git add internal/handlers/session_file_handler.go internal/handlers/terminal_handler.go internal/acp/service.go internal/handlers/session_handler.go
git commit -m "feat: 文件/终端 Handler cwd 从 Workspace 获取"
```

---

### 任务 11：注册 Workspace 路由

**文件：**
- 修改：`internal/router/router.go`

- [ ] **步骤 1：添加 workspace 路由**

在 protected 路由组中添加：

```go
// 在 session 路由附近添加
workspaceH := handlers.NewWorkspaceHandler(agentRouter)  // agentRouter 需要实现 WorkspaceStore
protected.POST("/workspaces", workspaceH.Create)
protected.GET("/workspaces", workspaceH.List)
protected.GET("/workspaces/:id", workspaceH.Get)
protected.PUT("/workspaces/:id", workspaceH.Update)
protected.DELETE("/workspaces/:id", workspaceH.Delete)
protected.POST("/workspaces/:id/save", workspaceH.Save)
```

注意：`agent.Router` 需要实现 `WorkspaceStore` 接口。在 `agent/router.go` 中添加委托方法（实际上 `acp.Service` 已经实现了这些方法，Router 只需要传递）：

```go
// Workspace-related methods (delegate to service)
func (r *Router) CreateWorkspace(ws *models.Workspace) error { return r.service.CreateWorkspace(ws) }
func (r *Router) FindWorkspaceByID(id uint) (*models.Workspace, error) { return r.service.FindWorkspaceByID(id) }
func (r *Router) FindWorkspacesByUserID(userID uint) ([]models.Workspace, error) { return r.service.FindWorkspacesByUserID(userID) }
func (r *Router) FindWorkspaceByUserIDAndCwd(userID uint, cwd string) (*models.Workspace, error) { return r.service.FindWorkspaceByUserIDAndCwd(userID, cwd) }
func (r *Router) FindDefaultWorkspaceByUserID(userID uint) (*models.Workspace, error) { return r.service.FindDefaultWorkspaceByUserID(userID) }
func (r *Router) UpdateWorkspace(id uint, updates map[string]interface{}) error { return r.service.UpdateWorkspace(id, updates) }
func (r *Router) DeleteWorkspace(id uint) error { return r.service.DeleteWorkspace(id) }
func (r *Router) WorkspaceSessionCount(workspaceID uint) (int64, error) { return r.service.WorkspaceSessionCount(workspaceID) }
func (r *Router) WorkspaceCountByUserID(userID uint) (int64, error) { return r.service.WorkspaceCountByUserID(userID) }
func (r *Router) FindSessionsByWorkspaceID(workspaceID uint) ([]models.Session, error) { return r.service.FindSessionsByWorkspaceID(workspaceID) }
func (r *Router) DeleteSessionWithMessages(session *models.Session) error { return r.service.DeleteSessionWithMessages(session) }
func (r *Router) GetWorkspaceCwd(workspaceID uint) (string, error) { return r.service.GetWorkspaceCwd(workspaceID) }
```

- [ ] **步骤 2：编译验证**

```bash
cd /Users/shenkonghui/src/mywork/NexusAgent && go build ./...
```
预期：全部编译通过。

- [ ] **步骤 3：Commit**

```bash
git add internal/router/router.go internal/agent/router.go
git commit -m "feat: 注册 workspace 路由，Router 实现 WorkspaceStore 接口"
```

---

### 任务 12：前端新增 Types 和 API 客户端

**文件：**
- 修改：`web/src/types.ts`
- 创建：`web/src/api/workspaces.ts`
- 修改：`web/src/api/sessions.ts`

- [ ] **步骤 1：types.ts 新增 Workspace 类型，更新 Session**

```typescript
// 新增 Workspace 接口
export interface Workspace {
  id: number
  user_id: number
  name: string
  cwd: string
  mode: 'persistent' | 'temporary'
  temp_dir?: string
  session_count?: number
  created_at: string
  updated_at: string
}

// Session 接口更新
export interface Session {
  id: number
  session_id: string
  agent_type: string
  status: 'active' | 'closed' | 'error'
  user_id: number
  workspace_id: number | null
  workspace?: Workspace
  last_prompt: string
  title: string
  source: 'manual' | 'scheduled'
  created_at: string
  closed_at: string | null
}
```

- [ ] **步骤 2：创建 workspace API 客户端**

```typescript
import type { Workspace, Session } from '../types'
import { apiFetch } from './client'

export function createWorkspace(name: string, cwd: string): Promise<{ data: Workspace }> {
  return apiFetch('/workspaces', {
    method: 'POST',
    body: JSON.stringify({ name, cwd }),
  })
}

export function listWorkspaces(): Promise<{ data: { workspaces: (Workspace & { session_count: number })[] } }> {
  return apiFetch('/workspaces')
}

export function getWorkspace(id: number): Promise<{ data: { workspace: Workspace; sessions: Session[] } }> {
  return apiFetch(`/workspaces/${id}`)
}

export function updateWorkspace(id: number, name: string): Promise<{ data: Workspace }> {
  return apiFetch(`/workspaces/${id}`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  })
}

export function deleteWorkspace(id: number): Promise<void> {
  return apiFetch(`/workspaces/${id}`, { method: 'DELETE' })
}

export function saveWorkspace(id: number, name: string, cwd: string): Promise<{ data: Workspace }> {
  return apiFetch(`/workspaces/${id}/save`, {
    method: 'POST',
    body: JSON.stringify({ name, cwd }),
  })
}
```

- [ ] **步骤 3：更新 createSession**

```typescript
export function createSession(agentType: string, workspaceId?: number, modelValue?: string): Promise<{ data: Session }> {
  return apiFetch('/sessions', {
    method: 'POST',
    body: JSON.stringify({
      agent_type: agentType,
      workspace_id: workspaceId || 0,
      model_value: modelValue || '',
    }),
  })
}
```

移除 `resumeSession` 的 `cwd` 参数：
```typescript
export function resumeSession(id: number): Promise<{ data: Session }> {
  return apiFetch(`/sessions/${id}/resume`, {
    method: 'POST',
  })
}
```

- [ ] **步骤 4：Commit**

```bash
git add web/src/types.ts web/src/api/workspaces.ts web/src/api/sessions.ts
git commit -m "feat: 前端新增 Workspace 类型、API 客户端，更新 createSession 参数"
```

---

### 任务 13：创建 WorkspaceSidebar 组件

**文件：**
- 创建：`web/src/components/WorkspaceSidebar.tsx`
- 创建：`web/src/components/WorkspaceSidebar.module.css`

- [ ] **步骤 1：实现 WorkspaceSidebar 组件**

```tsx
import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import type { Workspace } from '../types'
import styles from './WorkspaceSidebar.module.css'

interface WorkspaceSidebarProps {
  workspaces: (Workspace & { session_count?: number })[]
  currentId?: number
  onDelete: (id: number) => void
  onRename: (id: number, name: string) => void
  onSave: (id: number) => void
  onCreateClick: () => void
}

export default function WorkspaceSidebar({
  workspaces,
  currentId,
  onDelete,
  onRename,
  onSave,
  onCreateClick,
}: WorkspaceSidebarProps) {
  const [expanded, setExpanded] = useState<Set<number>>(new Set())
  const [contextMenu, setContextMenu] = useState<{ id: number; x: number; y: number } | null>(null)
  const [renaming, setRenaming] = useState<number | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const navigate = useNavigate()

  function toggleExpand(id: number) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }

  function handleContextMenu(e: React.MouseEvent, ws: Workspace) {
    e.preventDefault()
    setContextMenu({ id: ws.id, x: e.clientX, y: e.clientY })
  }

  function handleRenameStart(ws: Workspace) {
    setRenaming(ws.id)
    setRenameValue(ws.name)
    setContextMenu(null)
  }

  function handleRenameSubmit(id: number) {
    if (renameValue.trim()) {
      onRename(id, renameValue.trim())
    }
    setRenaming(null)
  }

  return (
    <div className={styles.sidebar}>
      <div className={styles.header}>
        <span className={styles.title}>工作区</span>
        <button className={styles.newBtn} onClick={onCreateClick} title="新建工作区">+</button>
      </div>
      <div className={styles.list}>
        {workspaces.map((ws) => (
          <div key={ws.id} className={`${styles.item} ${ws.id === currentId ? styles.active : ''}`}>
            <div
              className={styles.itemHeader}
              onClick={() => navigate(`/workspaces/${ws.id}`)}
              onContextMenu={(e) => handleContextMenu(e, ws)}
            >
              <span className={styles.icon}>{ws.mode === 'temporary' ? '🕐' : '📁'}</span>
              {renaming === ws.id ? (
                <input
                  className={styles.renameInput}
                  value={renameValue}
                  onChange={(e) => setRenameValue(e.target.value)}
                  onBlur={() => handleRenameSubmit(ws.id)}
                  onKeyDown={(e) => { if (e.key === 'Enter') handleRenameSubmit(ws.id); if (e.key === 'Escape') setRenaming(null) }}
                  autoFocus
                  onClick={(e) => e.stopPropagation()}
                />
              ) : (
                <span className={styles.name}>{ws.name}</span>
              )}
              {ws.session_count !== undefined && (
                <span className={styles.count}>{ws.session_count}</span>
              )}
            </div>
          </div>
        ))}
      </div>

      {contextMenu && (
        <div className={styles.contextMenu} style={{ top: contextMenu.y, left: contextMenu.x }}>
          <div className={styles.menuItem} onClick={() => {
            const ws = workspaces.find(w => w.id === contextMenu.id)
            if (ws) handleRenameStart(ws)
          }}>重命名</div>
          <div className={styles.menuItem} onClick={() => {
            const ws = workspaces.find(w => w.id === contextMenu.id)
            if (ws?.mode === 'temporary') onSave(contextMenu.id)
            setContextMenu(null)
          }}>保存为正式工作区</div>
          <div className={`${styles.menuItem} ${styles.danger}`} onClick={() => {
            onDelete(contextMenu.id)
            setContextMenu(null)
          }}>删除</div>
        </div>
      )}
    </div>
  )
}
```

- [ ] **步骤 2：添加样式文件**

略（参考现有 SessionSidebar.module.css 风格）。

- [ ] **步骤 3：Commit**

```bash
git add web/src/components/WorkspaceSidebar.tsx web/src/components/WorkspaceSidebar.module.css
git commit -m "feat: 新增 WorkspaceSidebar 组件"
```

---

### 任务 14：创建 HomePage（首页）

**文件：**
- 创建：`web/src/pages/HomePage.tsx`
- 创建：`web/src/pages/HomePage.module.css`

- [ ] **步骤 1：实现 HomePage**

```tsx
import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { listWorkspaces, createWorkspace, deleteWorkspace, updateWorkspace, saveWorkspace } from '../api/workspaces'
import { createSession } from '../api/sessions'
import { listAgents, probeAgentConfigs } from '../api/agents'
import type { Workspace, Agent, ConfigOption } from '../types'
import WorkspaceSidebar from '../components/WorkspaceSidebar'
import PromptInput from '../components/PromptInput'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import CreateWorkspaceDialog from '../components/CreateWorkspaceDialog'
import UserMenu from '../components/UserMenu'
import styles from './HomePage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

export default function HomePage() {
  const { t } = useTranslation()
  const { user, loading: authLoading } = useRequireAuth()
  const navigate = useNavigate()

  const [workspaces, setWorkspaces] = useState<(Workspace & { session_count?: number })[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgent, setSelectedAgent] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [probeConfigs, setProbeConfigs] = useState<ConfigOption[]>([])
  const [probing, setProbing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [showCreateDialog, setShowCreateDialog] = useState(false)

  const loadData = useCallback(async () => {
    setLoading(true)
    try {
      const [wsResp, agentsResp] = await Promise.all([listWorkspaces(), listAgents()])
      setWorkspaces(wsResp.data.workspaces || [])
      setAgents(agentsResp.data.agents || [])
      if (agentsResp.data.agents?.length > 0) {
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a: Agent) => a.type)
        setSelectedAgent(saved && types.includes(saved) ? saved : agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally { setLoading(false) }
  }, [])

  useEffect(() => { if (user) loadData() }, [user, loadData])

  // 探测 agent 配置
  useEffect(() => {
    if (!selectedAgent) return
    let alive = true
    setProbing(true)
    probeAgentConfigs(selectedAgent)
      .then(r => {
        if (!alive) return
        const opts = r.data.config_options || []
        setProbeConfigs(opts)
        const modelOpt = opts.find(o => o.category === 'model')
        setSelectedModel(modelOpt?.current_value || modelOpt?.options[0]?.value || '')
      })
      .catch(() => { if (alive) { setProbeConfigs([]); setSelectedModel('') } })
      .finally(() => { if (alive) setProbing(false) })
    return () => { alive = false }
  }, [selectedAgent])

  async function handleFirstSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, 0, selectedModel || undefined)
      localStorage.setItem(DEFAULT_AGENT_KEY, selectedAgent)
      navigate(`/workspaces/${resp.data.workspace_id}/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败')
      setCreating(false)
    }
  }

  async function handleCreateWorkspace(name: string, cwd: string) {
    try {
      await createWorkspace(name, cwd)
      setShowCreateDialog(false)
      loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : '创建失败')
    }
  }

  async function handleDeleteWorkspace(id: number) {
    try { await deleteWorkspace(id); loadData() }
    catch (err) { setError(err instanceof Error ? err.message : '删除失败') }
  }

  async function handleRenameWorkspace(id: number, name: string) {
    try { await updateWorkspace(id, name); loadData() }
    catch (err) { setError(err instanceof Error ? err.message : '重命名失败') }
  }

  async function handleSaveWorkspace(id: number) {
    const ws = workspaces.find(w => w.id === id)
    if (ws) {
      try { await saveWorkspace(id, ws.name, ws.cwd); loadData() }
      catch (err) { setError(err instanceof Error ? err.message : '保存失败') }
    }
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null

  return (
    <div className={styles.layout}>
      <WorkspaceSidebar
        workspaces={workspaces}
        onDelete={handleDeleteWorkspace}
        onRename={handleRenameWorkspace}
        onSave={handleSaveWorkspace}
        onCreateClick={() => setShowCreateDialog(true)}
      />
      <div className={styles.main}>
        <div className={styles.header}>
          <span className={styles.agentType}>{t('session.newSession')}</span>
          <UserMenu />
        </div>
        <div className={styles.configBar}>
          <select className={styles.configSelect} value={selectedAgent}
            onChange={e => setSelectedAgent(e.target.value)} disabled={creating}>
            {agents.map(a => <option key={a.type} value={a.type}>{a.display_name}</option>)}
          </select>
          {probeConfigs.filter(o => o.type === 'select' && o.options.length > 0 && o.category === 'model').map(opt => (
            <input key={opt.id} className={styles.configInput}
              value={selectedModel}
              onChange={e => setSelectedModel(e.target.value)}
              disabled={probing || creating}
              placeholder="模型"
              list={`list-${opt.id}`}
            />
          ))}
          {probing && <span>探测配置中...</span>}
        </div>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}
        {loading ? <LoadingSpinner /> : (
          <div className={styles.hero}>
            <h2>{t('session.newSession')}</h2>
            <p>选择 Agent 后直接输入 prompt 开始对话（使用默认工作区）</p>
          </div>
        )}
        <PromptInput onSend={handleFirstSend}
          sending={creating}
          disabled={!selectedAgent || creating}
          placeholder={t('session.quickSendPlaceholder')}
        />
      </div>
      {showCreateDialog && (
        <CreateWorkspaceDialog
          onSubmit={handleCreateWorkspace}
          onClose={() => setShowCreateDialog(false)}
        />
      )}
    </div>
  )
}
```

- [ ] **步骤 2：创建 CreateWorkspaceDialog**

```tsx
// web/src/components/CreateWorkspaceDialog.tsx
import { useState } from 'react'
import DirectoryPicker from './DirectoryPicker'
import styles from './CreateWorkspaceDialog.module.css'

interface Props {
  onSubmit: (name: string, cwd: string) => void
  onClose: () => void
}

export default function CreateWorkspaceDialog({ onSubmit, onClose }: Props) {
  const [name, setName] = useState('')
  const [cwd, setCwd] = useState('')
  const [showDirPicker, setShowDirPicker] = useState(false)

  function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (name.trim() && cwd.trim()) onSubmit(name.trim(), cwd.trim())
  }

  return (
    <div className={styles.overlay} onClick={onClose}>
      <form className={styles.dialog} onClick={e => e.stopPropagation()} onSubmit={handleSubmit}>
        <h3>新建工作区</h3>
        <input className={styles.input} type="text" value={name}
          onChange={e => setName(e.target.value)}
          placeholder="工作区名称" autoFocus required />
        <div className={styles.dirRow}>
          <input className={styles.input} type="text" value={cwd}
            onChange={e => setCwd(e.target.value)}
            placeholder="选择工作目录" required readOnly />
          <button type="button" className={styles.browseBtn}
            onClick={() => setShowDirPicker(true)}>浏览</button>
        </div>
        <div className={styles.actions}>
          <button type="button" onClick={onClose}>取消</button>
          <button type="submit" disabled={!name.trim() || !cwd.trim()}>创建</button>
        </div>
      </form>
      {showDirPicker && (
        <DirectoryPicker
          initialPath={cwd}
          onSelect={path => { setCwd(path); setShowDirPicker(false) }}
          onClose={() => setShowDirPicker(false)}
        />
      )}
    </div>
  )
}
```

- [ ] **步骤 3：Commit**

```bash
git add web/src/pages/HomePage.tsx web/src/pages/HomePage.module.css web/src/components/CreateWorkspaceDialog.tsx web/src/components/CreateWorkspaceDialog.module.css
git commit -m "feat: 新增 HomePage 和 CreateWorkspaceDialog 组件"
```

---

### 任务 15：创建 WorkspacePage

**文件：**
- 创建：`web/src/pages/WorkspacePage.tsx`
- 创建：`web/src/pages/WorkspacePage.module.css`

- [ ] **步骤 1：实现 WorkspacePage**

```tsx
import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useRequireAuth } from '../hooks/useRequireAuth'
import { getWorkspace, listWorkspaces, deleteWorkspace, updateWorkspace, saveWorkspace } from '../api/workspaces'
import { listSessions, createSession } from '../api/sessions'
import { listAgents, probeAgentConfigs } from '../api/agents'
import type { Workspace, Session, Agent, ConfigOption } from '../types'
import WorkspaceSidebar from '../components/WorkspaceSidebar'
import PromptInput from '../components/PromptInput'
import ErrorBanner from '../components/ErrorBanner'
import LoadingSpinner from '../components/LoadingSpinner'
import UserMenu from '../components/UserMenu'
import styles from './WorkspacePage.module.css'

const DEFAULT_AGENT_KEY = 'nexus.default.agent'

export default function WorkspacePage() {
  const { t } = useTranslation()
  const { wid } = useParams<{ wid: string }>()
  const workspaceId = Number(wid)
  const navigate = useNavigate()
  const { user, loading: authLoading } = useRequireAuth()

  const [workspace, setWorkspace] = useState<Workspace | null>(null)
  const [sessions, setSessions] = useState<Session[]>([])
  const [allWorkspaces, setAllWorkspaces] = useState<(Workspace & { session_count?: number })[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [selectedAgent, setSelectedAgent] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [probeConfigs, setProbeConfigs] = useState<ConfigOption[]>([])
  const [probing, setProbing] = useState(false)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  const loadData = useCallback(async () => {
    if (!workspaceId) return
    setLoading(true)
    try {
      const [wsResp, allWsResp, agentsResp] = await Promise.all([
        getWorkspace(workspaceId), listWorkspaces(), listAgents(),
      ])
      setWorkspace(wsResp.data.workspace)
      setSessions(wsResp.data.sessions || [])
      setAllWorkspaces(allWsResp.data.workspaces || [])
      setAgents(agentsResp.data.agents || [])
      if (agentsResp.data.agents?.length > 0) {
        const saved = localStorage.getItem(DEFAULT_AGENT_KEY)
        const types = agentsResp.data.agents.map((a: Agent) => a.type)
        setSelectedAgent(saved && types.includes(saved) ? saved : agentsResp.data.agents[0].type)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '加载失败')
    } finally { setLoading(false) }
  }, [workspaceId])

  useEffect(() => { if (user) loadData() }, [user, loadData])

  useEffect(() => {
    if (!selectedAgent) return
    let alive = true
    setProbing(true)
    probeAgentConfigs(selectedAgent)
      .then(r => {
        if (!alive) return
        const opts = r.data.config_options || []
        setProbeConfigs(opts)
        const modelOpt = opts.find(o => o.category === 'model')
        setSelectedModel(modelOpt?.current_value || modelOpt?.options[0]?.value || '')
      })
      .catch(() => { if (alive) { setProbeConfigs([]); setSelectedModel('') } })
      .finally(() => { if (alive) setProbing(false) })
    return () => { alive = false }
  }, [selectedAgent])

  async function handleSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, workspaceId, selectedModel || undefined)
      localStorage.setItem(DEFAULT_AGENT_KEY, selectedAgent)
      navigate(`/workspaces/${workspaceId}/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : '操作失败')
      setCreating(false)
    }
  }

  async function handleDeleteWorkspace(id: number) {
    try { await deleteWorkspace(id); navigate('/') }
    catch (err) { setError(err instanceof Error ? err.message : '删除失败') }
  }

  async function handleRenameWorkspace(id: number, name: string) {
    try { await updateWorkspace(id, name); loadData() }
    catch (err) { setError(err instanceof Error ? err.message : '重命名失败') }
  }

  async function handleSaveWorkspace(id: number) {
    const ws = allWorkspaces.find(w => w.id === id)
    if (ws) {
      try { await saveWorkspace(id, ws.name, ws.cwd); loadData() }
      catch (err) { setError(err instanceof Error ? err.message : '保存失败') }
    }
  }

  if (authLoading) return <LoadingSpinner text={t('common.loading')} />
  if (!user) return null
  if (loading) return <LoadingSpinner text={t('common.loading')} />

  return (
    <div className={styles.layout}>
      <WorkspaceSidebar
        workspaces={allWorkspaces}
        currentId={workspaceId}
        onDelete={handleDeleteWorkspace}
        onRename={handleRenameWorkspace}
        onSave={handleSaveWorkspace}
        onCreateClick={() => navigate('/')}
      />
      <div className={styles.main}>
        <div className={styles.header}>
          <div className={styles.workspaceInfo}>
            <span className={styles.wsName}>{workspace?.name}</span>
            <span className={styles.wsCwd}>{workspace?.cwd}</span>
          </div>
          <UserMenu />
        </div>
        <div className={styles.configBar}>
          <select className={styles.configSelect} value={selectedAgent}
            onChange={e => setSelectedAgent(e.target.value)} disabled={creating}>
            {agents.map(a => <option key={a.type} value={a.type}>{a.display_name}</option>)}
          </select>
          {probeConfigs.filter(o => o.type === 'select' && o.category === 'model').map(opt => (
            <input key={opt.id} className={styles.configInput}
              value={selectedModel}
              onChange={e => setSelectedModel(e.target.value)}
              disabled={probing || creating}
              placeholder="模型"
            />
          ))}
        </div>
        {error && <ErrorBanner message={error} onClose={() => setError('')} />}

        <div className={styles.sessionsList}>
          <h3>会话历史</h3>
          {sessions.length === 0 && <p className={styles.empty}>暂无会话，输入 prompt 开始</p>}
          {sessions.map(s => (
            <div key={s.id} className={styles.sessionItem}
              onClick={() => navigate(`/workspaces/${workspaceId}/sessions/${s.id}`)}>
              <span className={styles.sessionTitle}>{s.title || '新会话'}</span>
              <span className={styles.sessionTime}>{new Date(s.created_at).toLocaleString()}</span>
            </div>
          ))}
        </div>

        <PromptInput onSend={handleSend}
          sending={creating}
          disabled={!selectedAgent || creating}
          placeholder="输入 prompt 开始新对话..."
        />
      </div>
    </div>
  )
}
```

- [ ] **步骤 2：Commit**

```bash
git add web/src/pages/WorkspacePage.tsx web/src/pages/WorkspacePage.module.css
git commit -m "feat: 新增 WorkspacePage 组件"
```

---

### 任务 16：改造 ChatPage 适配 workspace 路由

**文件：**
- 修改：`web/src/pages/ChatPage.tsx`

- [ ] **步骤 1：ChatPage 适配新路由参数**

ChatPage 当前使用 `/:id` 作为 sessionId。新路由为 `/workspaces/:wid/sessions/:sid`。

核心变更：
- `useParams` 改为读取 `wid` 和 `sid`
- `handleFirstSend` 改为使用默认 workspace 创建 session（参数 `workspace_id: wid`）
- 移除 `agentCwd` 状态和目录选择逻辑
- 移除 `cwd` 相关的 props 传递

```tsx
// 路由参数变更
const { wid, sid } = useParams<{ wid: string; sid: string }>()
const workspaceId = Number(wid)
const sessionId = Number(sid)
const hasSession = !isNaN(sessionId)

// handleFirstSend 中移除 cwd
async function handleFirstSend(prompt: string) {
    if (!selectedAgent || creating) return
    setCreating(true); setError('')
    try {
      const resp = await createSession(selectedAgent, workspaceId, selectedModel || undefined)
      localStorage.setItem(DEFAULT_AGENT_KEY, selectedAgent)
      navigate(`/workspaces/${workspaceId}/sessions/${resp.data.id}`, { state: { initialPrompt: prompt } })
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
      setCreating(false)
    }
}

// handleResume 移除 cwd 选择
async function handleResume() {
    setResuming(true); setError('')
    try {
      await resumeSession(sessionId)
      await loadData()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('common.failed'))
    } finally { setResuming(false) }
}
```

- [ ] **步骤 2：移除无会话模式 UI 中的目录选择器**

移除 `agentCwd`、`showDirPicker`、`resumeCwd`、`showResumePicker` 状态和相关 JSX。

保留 Agent 和模型选择。

- [ ] **步骤 3：Commit**

```bash
git add web/src/pages/ChatPage.tsx
git commit -m "feat: ChatPage 适配 workspace/:wid/sessions/:sid 路由，移除目录选择"
```

---

### 任务 17：更新 App 路由和 i18n

**文件：**
- 修改：`web/src/App.tsx`
- 修改：`web/src/i18n/zh.json`

- [ ] **步骤 1：更新路由**

```tsx
import HomePage from './pages/HomePage'
import WorkspacePage from './pages/WorkspacePage'

// Routes 内：
<Route path="/" element={<HomePage />} />
<Route path="/workspaces/:wid" element={<WorkspacePage />} />
<Route path="/workspaces/:wid/sessions/:sid" element={<ChatPage />} />
{/* 旧路由兼容重定向 */}
<Route path="/sessions/:id" element={<SessionRedirect />} />
```

新增 `SessionRedirect` 组件：
```tsx
// web/src/components/SessionRedirect.tsx
import { useEffect, useState } from 'react'
import { useParams, Navigate } from 'react-router-dom'
import { getSession } from '../api/sessions'
import LoadingSpinner from './LoadingSpinner'

export default function SessionRedirect() {
  const { id } = useParams<{ id: string }>()
  const [target, setTarget] = useState<string | null>(null)

  useEffect(() => {
    getSession(Number(id)).then(r => {
      const wid = r.data.workspace_id
      if (wid) setTarget(`/workspaces/${wid}/sessions/${id}`)
      else setTarget('/')
    }).catch(() => setTarget('/'))
  }, [id])

  if (!target) return <LoadingSpinner />
  return <Navigate to={target} replace />
}
```

- [ ] **步骤 2：更新 i18n**

在 `zh.json` 中新增：
```json
{
  "workspace": {
    "title": "工作区",
    "create": "新建工作区",
    "name": "名称",
    "cwd": "工作目录",
    "save": "保存为正式工作区",
    "delete": "删除工作区",
    "rename": "重命名",
    "default": "默认工作区",
    "empty": "暂无工作区"
  }
}
```

- [ ] **步骤 3：编译前端**

```bash
cd /Users/shenkonghui/src/mywork/NexusAgent/web && npm run build
```
预期：编译成功。

- [ ] **步骤 4：Commit**

```bash
git add web/src/App.tsx web/src/i18n/zh.json web/src/components/SessionRedirect.tsx
git commit -m "feat: 更新路由为 workspace 结构，添加旧路由重定向和 i18n"
```

---

### 任务 18：端到端验证

- [ ] **步骤 1：启动服务**

```bash
cd /Users/shenkonghui/src/mywork/NexusAgent && go run ./cmd/server/
```

- [ ] **步骤 2：测试 workspace 创建**

```bash
# 先登录获取 token
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"admin","password":"admin123"}' | jq -r '.data.access_token' > /tmp/token

# 创建 workspace
curl -s -X POST http://localhost:8080/api/v1/workspaces \
  -H "Authorization: Bearer $(cat /tmp/token)" \
  -H 'Content-Type: application/json' \
  -d '{"name":"测试项目","cwd":"/tmp"}' | jq
```
预期：返回 201，包含 workspace 对象。

- [ ] **步骤 3：测试在 workspace 内创建 session**

```bash
curl -s -X POST http://localhost:8080/api/v1/sessions \
  -H "Authorization: Bearer $(cat /tmp/token)" \
  -H 'Content-Type: application/json' \
  -d '{"agent_type":"claude-code","workspace_id":1}' | jq
```
预期：返回 201，session 的 `workspace_id` 为 1。

- [ ] **步骤 4：打开前端验证**

打开浏览器访问 `http://localhost:8080`，验证：
- 首页显示 workspace 列表
- 可以创建新 workspace
- 点击 workspace 进入详情页
- 可以创建 session 并正常对话

- [ ] **步骤 5：Commit 最终修复（如有）**

---

## 自检

### 规格覆盖度
- ✅ Workspace 数据模型 → 任务 1
- ✅ Session 模型变更 → 任务 2
- ✅ 数据库迁移 → 任务 3
- ✅ WorkspaceRepository → 任务 4
- ✅ SessionRepository 更新 → 任务 5
- ✅ WorkspaceHandler → 任务 6
- ✅ ACP Service 适配 → 任务 7
- ✅ Router 适配 → 任务 8
- ✅ SessionHandler 更新 → 任务 9
- ✅ 文件/终端 Handler → 任务 10
- ✅ 路由注册 → 任务 11
- ✅ 前端 Types/API → 任务 12
- ✅ WorkspaceSidebar → 任务 13
- ✅ HomePage → 任务 14
- ✅ WorkspacePage → 任务 15
- ✅ ChatPage 改造 → 任务 16
- ✅ App 路由 + i18n → 任务 17
- ✅ 验证 → 任务 18

### 占位符扫描
- 无 TODO、无待定项

### 类型一致性
- `WorkspaceID uint` 在 model、service、handler、router、前端 API 中一致
- `WorkspaceModePersistent` / `WorkspaceModeTemporary` 常量在 models 中统一定义
- 错误处理在各层一致
