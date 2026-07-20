package repository

import (
	"gorm.io/gorm"

	"opennexus/internal/models"
)

// TaskExecutionRepository 管理定时任务执行记录的持久化。
type TaskExecutionRepository struct {
	db *gorm.DB
}

func NewTaskExecutionRepository(db *gorm.DB) *TaskExecutionRepository {
	return &TaskExecutionRepository{db: db}
}

// Create 创建一条执行记录。
func (r *TaskExecutionRepository) Create(e *models.TaskExecution) error {
	return r.db.Create(e).Error
}

// UpdateStatus 更新执行状态与错误信息，并设置 finished_at。
func (r *TaskExecutionRepository) UpdateStatus(id uint, status, errMsg string) error {
	return r.db.Model(&models.TaskExecution{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":      status,
			"error":       errMsg,
			"finished_at": gorm.Expr("CURRENT_TIMESTAMP"),
		}).Error
}

// FindByTaskExecutionID 查找指定任务下某个 execution_id 的执行记录。
func (r *TaskExecutionRepository) FindByTaskExecutionID(taskID, executionID uint) (*models.TaskExecution, error) {
	var e models.TaskExecution
	if err := r.db.Where("task_id = ? AND execution_id = ?", taskID, executionID).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

// ListByTaskID 列出指定任务的所有执行记录，按 execution_id 降序（最新优先）。
func (r *TaskExecutionRepository) ListByTaskID(taskID uint) ([]models.TaskExecution, error) {
	var list []models.TaskExecution
	err := r.db.Where("task_id = ?", taskID).Order("execution_id DESC").Find(&list).Error
	return list, err
}

// ListByTaskIDAndExecutionIDs 查询指定任务下给定 execution_id 集合的执行记录。
func (r *TaskExecutionRepository) ListByTaskIDAndExecutionIDs(taskID uint, execIDs []uint) ([]models.TaskExecution, error) {
	if len(execIDs) == 0 {
		return nil, nil
	}
	var list []models.TaskExecution
	err := r.db.Where("task_id = ? AND execution_id IN ?", taskID, execIDs).Find(&list).Error
	return list, err
}
