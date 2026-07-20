package repository

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"opennexus/internal/models"
)

var ErrScheduledTaskNotFound = errors.New("定时任务不存在")

// ScheduledTaskRepository 管理定时任务的持久化。
type ScheduledTaskRepository struct {
	db *gorm.DB
}

func NewScheduledTaskRepository(db *gorm.DB) *ScheduledTaskRepository {
	return &ScheduledTaskRepository{db: db}
}

// FindByUserID 返回指定用户的全部定时任务，按 ID 降序（最新优先）。
func (r *ScheduledTaskRepository) FindByUserID(userID uint) ([]models.ScheduledTask, error) {
	var list []models.ScheduledTask
	err := r.db.Where("user_id = ?", userID).Order("id DESC").Find(&list).Error
	return list, err
}

// FindByUserIDAndWorkspace 返回指定用户指定工作区下的定时任务。
// workspaceID 为 0 时等价于 FindByUserID。
func (r *ScheduledTaskRepository) FindByUserIDAndWorkspace(userID, workspaceID uint) ([]models.ScheduledTask, error) {
	var list []models.ScheduledTask
	q := r.db.Where("user_id = ?", userID)
	if workspaceID > 0 {
		q = q.Where("workspace_id = ?", workspaceID)
	}
	err := q.Order("id DESC").Find(&list).Error
	return list, err
}

// FindAllEnabled 返回所有启用的定时任务（调度器启动时加载）。
func (r *ScheduledTaskRepository) FindAllEnabled() ([]models.ScheduledTask, error) {
	var list []models.ScheduledTask
	err := r.db.Where("enabled = ?", true).Find(&list).Error
	return list, err
}

// FindByID 按主键查询。
func (r *ScheduledTaskRepository) FindByID(id uint) (*models.ScheduledTask, error) {
	var t models.ScheduledTask
	if err := r.db.First(&t, id).Error; err != nil {
		return nil, ErrScheduledTaskNotFound
	}
	return &t, nil
}

// Create 落库。
func (r *ScheduledTaskRepository) Create(t *models.ScheduledTask) error {
	return r.db.Create(t).Error
}

// Update 更新全部字段。
func (r *ScheduledTaskRepository) Update(t *models.ScheduledTask) error {
	return r.db.Save(t).Error
}

// Delete 按主键删除。
func (r *ScheduledTaskRepository) Delete(id uint) error {
	return r.db.Delete(&models.ScheduledTask{}, id).Error
}

// UpdateSessionRef 回填任务关联的会话信息（首次执行后调用）。
func (r *ScheduledTaskRepository) UpdateSessionRef(id uint, sessionID string, dbSessionID uint) error {
	return r.db.Model(&models.ScheduledTask{}).Where("id = ?", id).
		Updates(map[string]interface{}{"session_id": sessionID, "db_session_id": dbSessionID}).Error
}

// UpdateRunResult 更新最近一次执行结果。
func (r *ScheduledTaskRepository) UpdateRunResult(id uint, status, lastErr string, runAt time.Time) error {
	return r.db.Model(&models.ScheduledTask{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_status": status,
			"last_error":  lastErr,
			"last_run_at": runAt,
		}).Error
}
