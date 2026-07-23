package repository

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"opennexus/internal/models"
)

// ACPConnectionRepository 读写 acp_connections 心跳表，主 server 与 watchdog 共用。
type ACPConnectionRepository struct {
	db *gorm.DB
}

func NewACPConnectionRepository(db *gorm.DB) *ACPConnectionRepository {
	return &ACPConnectionRepository{db: db}
}

// Upsert 插入或更新指定连接的心跳记录（以 pool_key 为唯一键）。
// lastActive / heartbeat 均刷新为 now，供建连与续约使用。
func (r *ACPConnectionRepository) Upsert(poolKey, agentType, cwd string, pid int) error {
	now := time.Now()
	row := models.ACPConnection{
		PoolKey:        poolKey,
		AgentType:      agentType,
		Cwd:            cwd,
		Pid:            pid,
		LastActiveAt:   now,
		ServerHeartbeat: now,
	}
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "pool_key"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"agent_type", "cwd", "pid", "last_active_at", "server_heartbeat", "updated_at",
		}),
	}).Create(&row).Error
}

// TouchActivity 刷新指定连接的最后活动时间（发 prompt 时调用）。
func (r *ACPConnectionRepository) TouchActivity(poolKey string) error {
	return r.db.Model(&models.ACPConnection{}).
		Where("pool_key = ?", poolKey).
		Updates(map[string]interface{}{
			"last_active_at": time.Now(),
			"server_heartbeat": time.Now(),
		}).Error
}

// TouchHeartbeat 全表续约 server 心跳（主 server 周期性调用）。
func (r *ACPConnectionRepository) TouchHeartbeat() error {
	return r.db.Model(&models.ACPConnection{}).
		Where("1 = 1").
		Update("server_heartbeat", time.Now()).Error
}

// Delete 按 poolKey 删除一行（连接关闭 / 进程退出时调用）。
func (r *ACPConnectionRepository) Delete(poolKey string) error {
	return r.db.Where("pool_key = ?", poolKey).Delete(&models.ACPConnection{}).Error
}

// DeleteAll 清空全部心跳记录。
// 供主 server 启动时清理上次崩溃残留的脏行（PID 已失效，保留会误导 watchdog）。
// 正常运行时不应调用——连接的生命周期由 Delete(poolKey) 管理。
func (r *ACPConnectionRepository) DeleteAll() error {
	return r.db.Where("1 = 1").Delete(&models.ACPConnection{}).Error
}

// FindByPoolKey 按 poolKey 查询单条记录。
func (r *ACPConnectionRepository) FindByPoolKey(poolKey string) (*models.ACPConnection, error) {
	var row models.ACPConnection
	err := r.db.Where("pool_key = ?", poolKey).First(&row).Error
	return &row, err
}

// FindIdle 返回 last_active_at 早于 cutoff 的连接（供 watchdog 判定空闲回收）。
func (r *ACPConnectionRepository) FindIdle(cutoff time.Time) ([]models.ACPConnection, error) {
	var rows []models.ACPConnection
	err := r.db.Where("last_active_at < ?", cutoff).Find(&rows).Error
	return rows, err
}

// FindAll 返回全部心跳记录（供 watchdog 主程序死亡时批量清理）。
func (r *ACPConnectionRepository) FindAll() ([]models.ACPConnection, error) {
	var rows []models.ACPConnection
	err := r.db.Find(&rows).Error
	return rows, err
}

// MaxHeartbeat 返回表中最近的 server_heartbeat；表空时返回零值与 false。
// watchdog 据此判断主程序是否仍在运行。
func (r *ACPConnectionRepository) MaxHeartbeat() (time.Time, bool, error) {
	var row models.ACPConnection
	err := r.db.Order("server_heartbeat DESC").First(&row).Error
	if err == gorm.ErrRecordNotFound {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	return row.ServerHeartbeat, true, nil
}
