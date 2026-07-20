package repository

import (
	"errors"

	"gorm.io/gorm"

	"opennexus/internal/models"
)

var ErrUserNotFound = errors.New("用户不存在")

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Create(user *models.User) error {
	return r.db.Create(user).Error
}

func (r *UserRepository) FindByUsername(username string) (*models.User, error) {
	var u models.User
	if err := r.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

func (r *UserRepository) FindByEmail(email string) (*models.User, error) {
	var u models.User
	if err := r.db.Where("email = ?", email).First(&u).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

func (r *UserRepository) FindByID(id uint) (*models.User, error) {
	var u models.User
	if err := r.db.First(&u, id).Error; err != nil {
		return nil, ErrUserNotFound
	}
	return &u, nil
}

func (r *UserRepository) ExistsByUsernameOrEmail(username, email string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).
		Where("username = ? OR email = ?", username, email).
		Count(&count).Error
	return count > 0, err
}

// ExistsByUsernameOrEmailExcludingID 检查除指定用户外是否存在同名用户名或邮箱（用于更新时唯一性校验）。
func (r *UserRepository) ExistsByUsernameOrEmailExcludingID(id uint, username, email string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).
		Where("id <> ? AND (username = ? OR email = ?)", id, username, email).
		Count(&count).Error
	return count > 0, err
}

// UpdateProfile 更新用户名与邮箱。
func (r *UserRepository) UpdateProfile(id uint, username, email string) error {
	return r.db.Model(&models.User{}).Where("id = ?", id).
		Updates(map[string]interface{}{"username": username, "email": email}).Error
}

// UpdatePassword 更新密码哈希。
func (r *UserRepository) UpdatePassword(id uint, passwordHash string) error {
	return r.db.Model(&models.User{}).Where("id = ?", id).
		Update("password_hash", passwordHash).Error
}
