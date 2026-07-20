package repository

import (
	"encoding/json"
	"errors"
	"strings"

	"gorm.io/gorm"

	"opennexus/internal/models"
)

var ErrNoteNotFound = errors.New("笔记不存在")

// NoteRepository 管理笔记持久化。
type NoteRepository struct {
	db *gorm.DB
}

func NewNoteRepository(db *gorm.DB) *NoteRepository {
	return &NoteRepository{db: db}
}

// FindByUserID 返回用户全部匹配笔记，按更新时间降序；tag 非空时按标签过滤。
func (r *NoteRepository) FindByUserID(userID uint, tag string) ([]models.Note, error) {
	dbq := r.db.Where("user_id = ?", userID)
	if tag != "" {
		dbq = dbq.Where("tags LIKE ?", "%\""+tag+"\"%")
	}
	var list []models.Note
	err := dbq.Order("updated_at DESC").Find(&list).Error
	return list, err
}

// FindByUserIDPaged 分页查询用户笔记（updated_at 降序，最新在前）。
func (r *NoteRepository) FindByUserIDPaged(userID uint, tag, q string, page, limit int) ([]models.Note, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	dbq := r.db.Model(&models.Note{}).Where("user_id = ?", userID)
	if tag != "" {
		dbq = dbq.Where("tags LIKE ?", "%\""+tag+"\"%")
	}
	if q = strings.TrimSpace(q); q != "" {
		like := "%" + q + "%"
		dbq = dbq.Where("title LIKE ? OR content LIKE ?", like, like)
	}
	var total int64
	if err := dbq.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var list []models.Note
	err := dbq.Order("updated_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&list).Error
	return list, total, err
}

// FindByID 按主键查询。
func (r *NoteRepository) FindByID(id uint) (*models.Note, error) {
	var n models.Note
	if err := r.db.First(&n, id).Error; err != nil {
		return nil, ErrNoteNotFound
	}
	return &n, nil
}

// Create 落库。
func (r *NoteRepository) Create(n *models.Note) error {
	return r.db.Create(n).Error
}

// CreateBatch 批量创建笔记（事务）。
func (r *NoteRepository) CreateBatch(notes []*models.Note) error {
	if len(notes) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		for _, n := range notes {
			if err := tx.Create(n).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Update 更新全部字段。
func (r *NoteRepository) Update(n *models.Note) error {
	return r.db.Save(n).Error
}

// Delete 按主键删除。
func (r *NoteRepository) Delete(id uint) error {
	return r.db.Delete(&models.Note{}, id).Error
}

// FindAllByUserID 返回用户全部笔记（不含 tag 过滤），可指定升序。
func (r *NoteRepository) FindAllByUserID(userID uint) ([]models.Note, error) {
	var list []models.Note
	err := r.db.Where("user_id = ?", userID).Order("updated_at ASC").Find(&list).Error
	return list, err
}

// FindPendingClassify 返回待自动分类的笔记，按更新时间升序。
func (r *NoteRepository) FindPendingClassify(limit int) ([]models.Note, error) {
	if limit <= 0 {
		limit = 20
	}
	var list []models.Note
	err := r.db.Where("classify_pending = ?", true).
		Order("updated_at ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// CountClassifyProgress 返回用户笔记完成数（非 pending）与总数。
func (r *NoteRepository) CountClassifyProgress(userID uint) (done, total int64, err error) {
	if err = r.db.Model(&models.Note{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return 0, 0, err
	}
	var pending int64
	if err = r.db.Model(&models.Note{}).Where("user_id = ? AND classify_pending = ?", userID, true).Count(&pending).Error; err != nil {
		return 0, 0, err
	}
	return total - pending, total, nil
}

// ListTags 返回用户所有标签（去重、排序）。
func (r *NoteRepository) ListTags(userID uint) ([]string, error) {
	var notes []models.Note
	if err := r.db.Where("user_id = ?", userID).Select("tags").Find(&notes).Error; err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var tags []string
	for _, n := range notes {
		var arr []string
		if n.Tags == "" {
			continue
		}
		if err := json.Unmarshal([]byte(n.Tags), &arr); err != nil {
			continue
		}
		for _, t := range arr {
			if t == "" {
				continue
			}
			if _, ok := seen[t]; ok {
				continue
			}
			seen[t] = struct{}{}
			tags = append(tags, t)
		}
	}
	return tags, nil
}
