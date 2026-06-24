package services

import (
	"errors"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"nexusagent/internal/models"
	"nexusagent/internal/repository"
)

var (
	ErrWeakPassword = errors.New("密码强度不足")
	ErrUserExists   = errors.New("用户名或邮箱已存在")
	ErrInvalidCreds = errors.New("账号或密码错误")
	ErrUserDisabled = errors.New("用户已被禁用")
	ErrInvalidToken = errors.New("无效或已过期的令牌")
)

var (
	hasLetter = regexp.MustCompile(`[a-zA-Z]`)
	hasDigit  = regexp.MustCompile(`[0-9]`)
)

type AuthService struct {
	db         *gorm.DB
	users      *repository.UserRepository
	tokens     *repository.RefreshTokenRepository
	jwt        *JWTService
	bcryptCost int
}

func NewAuthService(db *gorm.DB, jwtSvc *JWTService, bcryptCost int) *AuthService {
	return &AuthService{
		db:         db,
		users:      repository.NewUserRepository(db),
		tokens:     repository.NewRefreshTokenRepository(db),
		jwt:        jwtSvc,
		bcryptCost: bcryptCost,
	}
}

func (s *AuthService) validatePassword(password string) bool {
	return len(password) >= 8 && hasLetter.MatchString(password) && hasDigit.MatchString(password)
}

func (s *AuthService) Register(username, email, password string) (*models.User, error) {
	username = strings.TrimSpace(username)
	email = strings.TrimSpace(email)

	if !s.validatePassword(password) {
		return nil, ErrWeakPassword
	}

	exists, err := s.users.ExistsByUsernameOrEmail(username, email)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUserExists
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		Username:     username,
		Email:        email,
		PasswordHash: string(hash),
		Role:         models.RoleUser,
		Status:       models.StatusActive,
	}
	if err := s.users.Create(user); err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, ErrUserExists
		}
		if isUniqueViolation(err) {
			return nil, ErrUserExists
		}
		return nil, err
	}
	return user, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "UNIQUE constraint failed")
}
