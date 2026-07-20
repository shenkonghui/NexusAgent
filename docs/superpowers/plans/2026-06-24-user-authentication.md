# 用户认证系统（P3）实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 为 openNexus 平台实现独立可用的用户认证系统（注册/登录/刷新/登出 + JWT 双令牌 + RBAC 中间件）。

**架构：** 分层 Go 服务（Gin + GORM + SQLite）。handler → service → repository → model 四层，中间件做认证拦截。Access Token 无状态 JWT（15m），Refresh Token 落库可吊销（7d），刷新采用轮换 + 重放检测。

**技术栈：** Go、Gin、GORM、SQLite、golang-jwt/jwt/v5、golang.org/x/crypto/bcrypt、google/uuid、yaml.v3

**对应规格：** `docs/superpowers/specs/2026-06-24-user-authentication-design.md`

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `go.mod` / `go.sum` | 模块定义与依赖 |
| `config.yaml` | 默认配置 |
| `cmd/server/main.go` | 程序入口，装配依赖、校验配置、启动 HTTP |
| `internal/config/config.go` | 配置加载（yaml + 环境变量覆盖） |
| `internal/config/config_test.go` | 配置加载测试 |
| `internal/database/database.go` | SQLite 连接、GORM 迁移 |
| `internal/database/database_test.go` | 数据库连接测试 |
| `internal/models/user.go` | User GORM 模型 |
| `internal/models/refresh_token.go` | RefreshToken GORM 模型 |
| `internal/repository/user_repository.go` | 用户数据访问 |
| `internal/repository/user_repository_test.go` | 用户仓库测试 |
| `internal/repository/refresh_token_repository.go` | 令牌数据访问 |
| `internal/repository/refresh_token_repository_test.go` | 令牌仓库测试 |
| `internal/services/jwt_service.go` | JWT 签发/校验 |
| `internal/services/jwt_service_test.go` | JWT 服务测试 |
| `internal/services/auth_service.go` | 认证业务逻辑 |
| `internal/services/auth_service_test.go` | 认证业务测试 |
| `internal/handlers/response.go` | 统一响应辅助 |
| `internal/handlers/auth_handler.go` | 认证 HTTP handler |
| `internal/handlers/auth_handler_test.go` | handler 集成测试 |
| `internal/middleware/auth_middleware.go` | AuthRequired / RequireRole |
| `internal/middleware/auth_middleware_test.go` | 中间件测试 |
| `internal/router/router.go` | 路由注册 |

**包名约定：** 目录名作包名（`config`、`database`、`models`、`repository`、`services`、`handlers`、`middleware`、`router`）。模块名 `opennexus`。

**测试约定：** 单元测试用内存 SQLite（DSN `file::memory:?cache=shared`），handler 测试用 `httptest` + Gin test mode。运行测试命令：`go test ./...`。

---

## 任务 1：项目初始化与依赖

**文件：**
- 创建：`go.mod`
- 创建：`config.yaml`
- 创建：`.gitignore`（已存在，检查即可）

- [ ] **步骤 1：初始化模块**

运行：
```bash
go mod init opennexus
```

- [ ] **步骤 2：添加依赖**

运行：
```bash
go get github.com/gin-gonic/gin@latest
go get gorm.io/gorm@latest
go get gorm.io/driver/sqlite@latest
go get github.com/golang-jwt/jwt/v5@latest
go get golang.org/x/crypto/bcrypt
go get github.com/google/uuid@latest
go get gopkg.in/yaml.v3@latest
go mod tidy
```

- [ ] **步骤 3：创建默认配置文件**

创建 `config.yaml`：
```yaml
server:
  port: 8080
  mode: debug
database:
  path: ./data/opennexus.db
jwt:
  secret: ""
  access_ttl: 15m
  refresh_ttl: 168h
password:
  bcrypt_cost: 12
```

- [ ] **步骤 4：验证可编译**

运行：`go build ./...`
预期：无错误（无源码时也应通过）。

- [ ] **步骤 5：Commit**

```bash
git add go.mod go.sum config.yaml
git commit -m "chore: 初始化 Go 模块与依赖"
```

---

## 任务 2：配置加载

**文件：**
- 创建：`internal/config/config.go`
- 测试：`internal/config/config_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/config/config_test.go`：
```go
package config

import (
	"os"
	"testing"
	"time"
)

func TestLoad_FromYAML(t *testing.T) {
	cfg, err := Load("testdata/config_test.yaml")
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Server.Port = %d, 期望 9090", cfg.Server.Port)
	}
	if cfg.Database.Path != "./data/test.db" {
		t.Errorf("Database.Path = %q, 期望 ./data/test.db", cfg.Database.Path)
	}
	if cfg.JWT.AccessTTL != 15*time.Minute {
		t.Errorf("JWT.AccessTTL = %v, 期望 15m", cfg.JWT.AccessTTL)
	}
	if cfg.Password.BcryptCost != 10 {
		t.Errorf("Password.BcryptCost = %d, 期望 10", cfg.Password.BcryptCost)
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("JWT_SECRET", "env-secret-from-env-var-long-enough")
	t.Setenv("SERVER_PORT", "7070")
	cfg, err := Load("testdata/config_test.yaml")
	if err != nil {
		t.Fatalf("Load 返回错误: %v", err)
	}
	if cfg.JWT.Secret != "env-secret-from-env-var-long-enough" {
		t.Errorf("JWT.Secret 未被环境变量覆盖: %q", cfg.JWT.Secret)
	}
	if cfg.Server.Port != 7070 {
		t.Errorf("Server.Port 未被环境变量覆盖: %d", cfg.Server.Port)
	}
}

func TestValidate_SecretTooShort(t *testing.T) {
	cfg := &Config{JWT: JWTConfig{Secret: "short"}}
	if err := cfg.Validate(); err == nil {
		t.Error("期望 secret 过短时返回错误，实际无错误")
	}
}

func TestValidate_OK(t *testing.T) {
	cfg := &Config{JWT: JWTConfig{Secret: "this-is-a-very-long-jwt-secret-key-32+bytes!"}}
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望校验通过，实际错误: %v", err)
	}
}
```

创建测试数据文件 `internal/config/testdata/config_test.yaml`：
```yaml
server:
  port: 9090
  mode: debug
database:
  path: ./data/test.db
jwt:
  secret: ""
  access_ttl: 15m
  refresh_ttl: 168h
password:
  bcrypt_cost: 10
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/config/...`
预期：FAIL，编译错误（`Load`、`Config` 等未定义）。

- [ ] **步骤 3：编写实现**

创建 `internal/config/config.go`：
```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	JWT      JWTConfig      `yaml:"jwt"`
	Password PasswordConfig `yaml:"password"`
}

type ServerConfig struct {
	Port int    `yaml:"port"`
	Mode string `yaml:"mode"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

type JWTConfig struct {
	Secret     string        `yaml:"secret"`
	AccessTTL  time.Duration `yaml:"access_ttl"`
	RefreshTTL time.Duration `yaml:"refresh_ttl"`
}

type PasswordConfig struct {
	BcryptCost int `yaml:"bcrypt_cost"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件: %w", err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件: %w", err)
	}
	cfg.applyEnv()
	return cfg, nil
}

func (c *Config) applyEnv() {
	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JWT.Secret = v
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			c.Server.Port = port
		}
	}
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		c.Database.Path = v
	}
}

func (c *Config) Validate() error {
	if len(c.JWT.Secret) < 32 {
		return fmt.Errorf("JWT_SECRET 长度必须 >= 32 字节，当前 %d", len(c.JWT.Secret))
	}
	if c.JWT.AccessTTL <= 0 {
		c.JWT.AccessTTL = 15 * time.Minute
	}
	if c.JWT.RefreshTTL <= 0 {
		c.JWT.RefreshTTL = 168 * time.Hour
	}
	if c.Password.BcryptCost == 0 {
		c.Password.BcryptCost = 12
	}
	return nil
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/config/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/config/ go.mod go.sum
git commit -m "feat(config): 添加配置加载与校验"
```

---

## 任务 3：数据库连接与迁移

**文件：**
- 创建：`internal/database/database.go`
- 测试：`internal/database/database_test.go`
- 创建：`internal/models/user.go`
- 创建：`internal/models/refresh_token.go`

- [ ] **步骤 1：编写模型**

创建 `internal/models/user.go`：
```go
package models

import "time"

const (
	RoleAdmin = "admin"
	RoleUser  = "user"

	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
	Email        string    `gorm:"uniqueIndex;size:255;not null" json:"email"`
	PasswordHash string    `gorm:"size:255;not null" json:"-"`
	Role         string    `gorm:"size:32;not null;default:user" json:"role"`
	Status       string    `gorm:"size:32;not null;default:active" json:"status"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	DeletedAt    *time.Time `gorm:"index" json:"-"`
}
```

创建 `internal/models/refresh_token.go`：
```go
package models

import "time"

type RefreshToken struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `gorm:"index;not null" json:"user_id"`
	TokenID   string    `gorm:"uniqueIndex;size:64;not null" json:"token_id"`
	ExpiresAt time.Time `gorm:"not null" json:"expires_at"`
	Revoked   bool      `gorm:"not null;default:false" json:"revoked"`
	UserAgent string    `gorm:"size:255" json:"user_agent"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}
```

- [ ] **步骤 2：编写失败的测试**

创建 `internal/database/database_test.go`：
```go
package database

import (
	"testing"

	"opennexus/models"
)

func TestConnect_MigratesTables(t *testing.T) {
	db, err := Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("Connect 返回错误: %v", err)
	}

	if !db.Migrator().HasTable(&models.User{}) {
		t.Error("期望 users 表已迁移，实际不存在")
	}
	if !db.Migrator().HasTable(&models.RefreshToken{}) {
		t.Error("期望 refresh_tokens 表已迁移，实际不存在")
	}
}
```

- [ ] **步骤 3：运行测试验证失败**

运行：`go test ./internal/database/...`
预期：FAIL，编译错误（`Connect` 未定义）。

- [ ] **步骤 4：编写实现**

创建 `internal/database/database.go`：
```go
package database

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"opennexus/models"
)

func Connect(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("打开数据库: %w", err)
	}
	if err := db.AutoMigrate(&models.User{}, &models.RefreshToken{}); err != nil {
		return nil, fmt.Errorf("迁移数据库: %w", err)
	}
	return db, nil
}
```

- [ ] **步骤 5：运行测试验证通过**

运行：`go test ./internal/database/...`
预期：PASS。

- [ ] **步骤 6：Commit**

```bash
git add internal/database/ internal/models/
git commit -m "feat(database): 添加 SQLite 连接与模型迁移"
```

---

## 任务 4：用户仓库

**文件：**
- 创建：`internal/repository/user_repository.go`
- 测试：`internal/repository/user_repository_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/repository/user_repository_test.go`：
```go
package repository

import (
	"testing"

	"gorm.io/gorm"

	"opennexus/database"
	"opennexus/models"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试数据库失败: %v", err)
	}
	// 清空表，保证测试隔离
	db.Exec("DELETE FROM users")
	db.Exec("DELETE FROM refresh_tokens")
	return db
}

func TestUserRepo_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &models.User{Username: "alice", Email: "alice@example.com", PasswordHash: "hash", Role: models.RoleUser, Status: models.StatusActive}
	if err := repo.Create(user); err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}
	if user.ID == 0 {
		t.Error("期望创建后 ID 非零")
	}
}

func TestUserRepo_DuplicateUsername(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	_ = repo.Create(&models.User{Username: "bob", Email: "bob1@example.com", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive})

	err := repo.Create(&models.User{Username: "bob", Email: "bob2@example.com", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive})
	if err == nil {
		t.Error("期望重复用户名返回错误")
	}
}

func TestUserRepo_FindByUsername(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	_ = repo.Create(&models.User{Username: "carol", Email: "carol@example.com", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive})

	got, err := repo.FindByUsername("carol")
	if err != nil {
		t.Fatalf("FindByUsername 返回错误: %v", err)
	}
	if got.Email != "carol@example.com" {
		t.Errorf("Email = %q", got.Email)
	}
}

func TestUserRepo_FindByUsername_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	if _, err := repo.FindByUsername("nobody"); err == nil {
		t.Error("期望未找到时返回错误")
	}
}

func TestUserRepo_FindByEmail(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	_ = repo.Create(&models.User{Username: "dave", Email: "dave@example.com", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive})

	got, err := repo.FindByEmail("dave@example.com")
	if err != nil {
		t.Fatalf("FindByEmail 返回错误: %v", err)
	}
	if got.Username != "dave" {
		t.Errorf("Username = %q", got.Username)
	}
}

func TestUserRepo_FindByID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)
	u := &models.User{Username: "eve", Email: "eve@example.com", PasswordHash: "h", Role: models.RoleUser, Status: models.StatusActive}
	_ = repo.Create(u)

	got, err := repo.FindByID(u.ID)
	if err != nil {
		t.Fatalf("FindByID 返回错误: %v", err)
	}
	if got.Username != "eve" {
		t.Errorf("Username = %q", got.Username)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/repository/...`
预期：FAIL，编译错误（`NewUserRepository` 未定义）。

- [ ] **步骤 3：编写实现**

创建 `internal/repository/user_repository.go`：
```go
package repository

import (
	"errors"

	"gorm.io/gorm"

	"opennexus/models"
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
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/repository/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/repository/
git commit -m "feat(repository): 添加用户仓库"
```

---

## 任务 5：RefreshToken 仓库

**文件：**
- 创建：`internal/repository/refresh_token_repository.go`
- 测试：`internal/repository/refresh_token_repository_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `internal/repository/refresh_token_repository_test.go`：
```go
package repository

import (
	"testing"
	"time"

	"opennexus/models"
)

func TestRefreshTokenRepo_CreateAndFindByJTI(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)

	rt := &models.RefreshToken{UserID: 1, TokenID: "jti-1", ExpiresAt: time.Now().Add(time.Hour), Revoked: false}
	if err := repo.Create(rt); err != nil {
		t.Fatalf("Create 返回错误: %v", err)
	}

	got, err := repo.FindByJTI("jti-1")
	if err != nil {
		t.Fatalf("FindByJTI 返回错误: %v", err)
	}
	if got.Revoked {
		t.Error("期望未吊销")
	}
}

func TestRefreshTokenRepo_FindByJTI_NotFound(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)
	if _, err := repo.FindByJTI("missing"); err == nil {
		t.Error("期望未找到返回错误")
	}
}

func TestRefreshTokenRepo_Revoke(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)
	_ = repo.Create(&models.RefreshToken{UserID: 1, TokenID: "jti-2", ExpiresAt: time.Now().Add(time.Hour)})

	if err := repo.Revoke("jti-2"); err != nil {
		t.Fatalf("Revoke 返回错误: %v", err)
	}
	got, _ := repo.FindByJTI("jti-2")
	if !got.Revoked {
		t.Error("期望已吊销")
	}
}

func TestRefreshTokenRepo_RevokeAllByUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewRefreshTokenRepository(db)
	_ = repo.Create(&models.RefreshToken{UserID: 7, TokenID: "a", ExpiresAt: time.Now().Add(time.Hour)})
	_ = repo.Create(&models.RefreshToken{UserID: 7, TokenID: "b", ExpiresAt: time.Now().Add(time.Hour)})
	_ = repo.Create(&models.RefreshToken{UserID: 9, TokenID: "c", ExpiresAt: time.Now().Add(time.Hour)})

	if err := repo.RevokeAllByUser(7); err != nil {
		t.Fatalf("RevokeAllByUser 返回错误: %v", err)
	}
	for _, jti := range []string{"a", "b"} {
		got, _ := repo.FindByJTI(jti)
		if !got.Revoked {
			t.Errorf("期望 %s 已吊销", jti)
		}
	}
	c, _ := repo.FindByJTI("c")
	if c.Revoked {
		t.Error("不应吊销其他用户的令牌")
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/repository/...`
预期：FAIL，编译错误（`NewRefreshTokenRepository` 未定义）。

- [ ] **步骤 3：编写实现**

创建 `internal/repository/refresh_token_repository.go`：
```go
package repository

import (
	"errors"

	"gorm.io/gorm"

	"opennexus/models"
)

var ErrTokenNotFound = errors.New("令牌不存在")

type RefreshTokenRepository struct {
	db *gorm.DB
}

func NewRefreshTokenRepository(db *gorm.DB) *RefreshTokenRepository {
	return &RefreshTokenRepository{db: db}
}

func (r *RefreshTokenRepository) Create(rt *models.RefreshToken) error {
	return r.db.Create(rt).Error
}

func (r *RefreshTokenRepository) FindByJTI(jti string) (*models.RefreshToken, error) {
	var rt models.RefreshToken
	if err := r.db.Where("token_id = ?", jti).First(&rt).Error; err != nil {
		return nil, ErrTokenNotFound
	}
	return &rt, nil
}

func (r *RefreshTokenRepository) Revoke(jti string) error {
	return r.db.Model(&models.RefreshToken{}).
		Where("token_id = ?", jti).
		Update("revoked", true).Error
}

func (r *RefreshTokenRepository) RevokeAllByUser(userID uint) error {
	return r.db.Model(&models.RefreshToken{}).
		Where("user_id = ?", userID).
		Update("revoked", true).Error
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/repository/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/repository/
git commit -m "feat(repository): 添加 RefreshToken 仓库"
```

---

## 任务 6：JWT 服务

**文件：**
- 创建：`internal/services/jwt_service.go`
- 测试：`internal/services/jwt_service_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/services/jwt_service_test.go`：
```go
package services

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "this-is-a-very-long-jwt-secret-key-32+bytes!"

func TestJWTService_GenerateAccess(t *testing.T) {
	svc := NewJWTService(testSecret, 15*time.Minute, time.Hour)
	token, err := svc.GenerateAccessToken(42, "alice", "admin")
	if err != nil {
		t.Fatalf("GenerateAccessToken 错误: %v", err)
	}
	if token == "" {
		t.Fatal("期望非空 token")
	}

	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse 错误: %v", err)
	}
	if claims.UserID != 42 {
		t.Errorf("UserID = %d", claims.UserID)
	}
	if claims.Username != "alice" {
		t.Errorf("Username = %q", claims.Username)
	}
	if claims.Role != "admin" {
		t.Errorf("Role = %q", claims.Role)
	}
	if claims.TokenType != "access" {
		t.Errorf("TokenType = %q", claims.TokenType)
	}
}

func TestJWTService_GenerateRefresh(t *testing.T) {
	svc := NewJWTService(testSecret, 15*time.Minute, time.Hour)
	token, jti, err := svc.GenerateRefreshToken(42)
	if err != nil {
		t.Fatalf("错误: %v", err)
	}
	if jti == "" {
		t.Error("期望非空 jti")
	}
	claims, err := svc.Parse(token)
	if err != nil {
		t.Fatalf("Parse 错误: %v", err)
	}
	if claims.TokenType != "refresh" {
		t.Errorf("TokenType = %q", claims.TokenType)
	}
	if claims.JTI != jti {
		t.Errorf("JTI 不匹配")
	}
}

func TestJWTService_Parse_Expired(t *testing.T) {
	svc := NewJWTService(testSecret, -1*time.Minute, time.Hour) // 已过期
	token, _, _ := svc.GenerateAccessToken(1, "u", "user")
	if _, err := svc.Parse(token); err == nil {
		t.Error("期望过期 token 返回错误")
	}
}

func TestJWTService_Parse_WrongSecret(t *testing.T) {
	svc := NewJWTService(testSecret, 15*time.Minute, time.Hour)
	token, _, _ := svc.GenerateAccessToken(1, "u", "user")
	other := NewJWTService("another-long-secret-key-that-is-32+bytes-ok!", 15*time.Minute, time.Hour)
	if _, err := other.Parse(token); err == nil {
		t.Error("期望错误密钥校验失败")
	}
}

// 确保引用 jwt 包以避免未使用 import
var _ = jwt.ErrTokenExpired
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/services/...`
预期：FAIL，编译错误（`NewJWTService`、`Claims` 未定义）。

- [ ] **步骤 3：编写实现**

创建 `internal/services/jwt_service.go`：
```go
package services

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

type Claims struct {
	UserID    uint   `json:"uid"`
	Username  string `json:"usr,omitempty"`
	Role      string `json:"rol,omitempty"`
	JTI       string `json:"jti,omitempty"`
	TokenType string `json:"ttype"`
	jwt.RegisteredClaims
}

type JWTService struct {
	secret     []byte
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewJWTService(secret string, accessTTL, refreshTTL time.Duration) *JWTService {
	return &JWTService{
		secret:     []byte(secret),
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (s *JWTService) GenerateAccessToken(userID uint, username, role string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:    userID,
		Username:  username,
		Role:      role,
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.accessTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *JWTService) GenerateRefreshToken(userID uint) (string, string, error) {
	now := time.Now()
	jti := uuid.NewString()
	claims := Claims{
		UserID:    userID,
		JTI:       jti,
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(s.refreshTTL)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(s.secret)
	if err != nil {
		return "", "", err
	}
	return signed, jti, nil
}

func (s *JWTService) Parse(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("非预期签名方法")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("无效令牌")
	}
	return claims, nil
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/services/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/services/
git commit -m "feat(services): 添加 JWT 服务"
```

---

## 任务 7：认证服务 — 注册

**文件：**
- 修改：`internal/services/auth_service.go`（创建）
- 测试：`internal/services/auth_service_test.go`（创建）

> 说明：本任务与任务 8-10 共用 `auth_service.go`，逐个能力增量实现。

- [ ] **步骤 1：编写失败的测试**

创建 `internal/services/auth_service_test.go`：
```go
package services

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"opennexus/database"
	"opennexus/models"
)

func newAuthSvc(t *testing.T) (*AuthService, *gorm.DB) {
	t.Helper()
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM users")
	db.Exec("DELETE FROM refresh_tokens")
	jwtSvc := NewJWTService("this-is-a-very-long-jwt-secret-key-32+bytes!", 15*time.Minute, time.Hour)
	svc := NewAuthService(db, jwtSvc, 10) // bcrypt cost=10 加速测试
	return svc, db
}

func TestAuthService_Register_Success(t *testing.T) {
	svc, db := newAuthSvc(t)
	user, err := svc.Register("alice", "alice@example.com", "Password123")
	if err != nil {
		t.Fatalf("Register 错误: %v", err)
	}
	if user.ID == 0 {
		t.Error("期望 ID 非零")
	}
	if user.PasswordHash == "Password123" {
		t.Error("密码不应明文存储")
	}
	// 校验确实落库
	var count int64
	db.Model(&models.User{}).Count(&count)
	if count != 1 {
		t.Errorf("期望 1 条用户，实际 %d", count)
	}
}

func TestAuthService_Register_WeakPassword(t *testing.T) {
	svc, _ := newAuthSvc(t)
	if _, err := svc.Register("bob", "bob@example.com", "short"); err != ErrWeakPassword {
		t.Errorf("期望 ErrWeakPassword，实际 %v", err)
	}
}

func TestAuthService_Register_DuplicateUsername(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("carol", "carol@example.com", "Password123")
	if _, err := svc.Register("carol", "other@example.com", "Password123"); err != ErrUserExists {
		t.Errorf("期望 ErrUserExists，实际 %v", err)
	}
}

func TestAuthService_Register_DuplicateEmail(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("dave", "dave@example.com", "Password123")
	if _, err := svc.Register("dave2", "dave@example.com", "Password123"); err != ErrUserExists {
		t.Errorf("期望 ErrUserExists，实际 %v", err)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/services/...`
预期：FAIL，编译错误（`AuthService`、`ErrWeakPassword`、`ErrUserExists`、`NewAuthService` 未定义）。

- [ ] **步骤 3：编写实现**

创建 `internal/services/auth_service.go`：
```go
package services

import (
	"errors"
	"regexp"
	"strings"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"opennexus/models"
	"opennexus/repository"
)

var (
	ErrWeakPassword   = errors.New("密码强度不足")
	ErrUserExists     = errors.New("用户名或邮箱已存在")
	ErrInvalidCreds   = errors.New("账号或密码错误")
	ErrUserDisabled   = errors.New("用户已被禁用")
	ErrInvalidToken   = errors.New("无效或已过期的令牌")
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
		// SQLite 唯一约束冲突也归为已存在
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
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/services/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/services/
git commit -m "feat(services): 添加用户注册与密码校验"
```

---

## 任务 8：认证服务 — 登录

**文件：**
- 修改：`internal/services/auth_service.go`
- 修改：`internal/services/auth_service_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `auth_service_test.go` 追加：
```go
func TestAuthService_Login_ByUsername(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("alice", "alice@example.com", "Password123")

	result, err := svc.Login("alice", "Password123", "ua", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login 错误: %v", err)
	}
	if result.AccessToken == "" || result.RefreshToken == "" {
		t.Error("期望非空令牌")
	}
	if result.User.Username != "alice" {
		t.Errorf("Username = %q", result.User.Username)
	}
}

func TestAuthService_Login_ByEmail(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("bob", "bob@example.com", "Password123")

	result, err := svc.Login("bob@example.com", "Password123", "ua", "127.0.0.1")
	if err != nil {
		t.Fatalf("Login 错误: %v", err)
	}
	if result.User.Username != "bob" {
		t.Errorf("Username = %q", result.User.Username)
	}
}

func TestAuthService_Login_WrongPassword(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("carol", "carol@example.com", "Password123")

	if _, err := svc.Login("carol", "WrongPass1", "ua", "127.0.0.1"); err != ErrInvalidCreds {
		t.Errorf("期望 ErrInvalidCreds，实际 %v", err)
	}
}

func TestAuthService_Login_UnknownAccount(t *testing.T) {
	svc, _ := newAuthSvc(t)
	if _, err := svc.Login("ghost", "Password123", "ua", "127.0.0.1"); err != ErrInvalidCreds {
		t.Errorf("期望 ErrInvalidCreds，实际 %v", err)
	}
}

func TestAuthService_Login_DisabledUser(t *testing.T) {
	svc, db := newAuthSvc(t)
	u, _ := svc.Register("dave", "dave@example.com", "Password123")
	db.Model(&models.User{}).Where("id = ?", u.ID).Update("status", models.StatusDisabled)

	if _, err := svc.Login("dave", "Password123", "ua", "127.0.0.1"); err != ErrUserDisabled {
		t.Errorf("期望 ErrUserDisabled，实际 %v", err)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/services/...`
预期：FAIL，`Login` / `AuthResult` 未定义。

- [ ] **步骤 3：编写实现**

在 `auth_service.go` 追加：
```go
import (
	// 已有 import 之外追加：
	"time"
)

type AuthResult struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	ExpiresIn    int64        `json:"expires_in"`
	User         *models.User `json:"user"`
}

func (s *AuthService) Login(account, password, userAgent, ip string) (*AuthResult, error) {
	user, err := s.findUserByAccount(account)
	if err != nil {
		return nil, ErrInvalidCreds // 统一错误，防用户枚举
	}
	if user.Status == models.StatusDisabled {
		return nil, ErrUserDisabled
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCreds
	}
	return s.issueTokens(user, userAgent, ip)
}

func (s *AuthService) findUserByAccount(account string) (*models.User, error) {
	if strings.Contains(account, "@") {
		return s.users.FindByEmail(account)
	}
	return s.users.FindByUsername(account)
}

func (s *AuthService) issueTokens(user *models.User, userAgent, ip string) (*AuthResult, error) {
	access, err := s.jwt.GenerateAccessToken(user.ID, user.Username, user.Role)
	if err != nil {
		return nil, err
	}
	refresh, jti, err := s.jwt.GenerateRefreshToken(user.ID)
	if err != nil {
		return nil, err
	}
	rt := &models.RefreshToken{
		UserID:    user.ID,
		TokenID:   jti,
		ExpiresAt: time.Now().Add(s.jwt.refreshTTL),
		UserAgent: userAgent,
		IP:        ip,
	}
	if err := s.tokens.Create(rt); err != nil {
		return nil, err
	}
	return &AuthResult{
		AccessToken:  access,
		RefreshToken: refresh,
		ExpiresIn:    int64(s.jwt.accessTTL.Seconds()),
		User:         user,
	}, nil
}
```

> 注意：`refreshTTL` 字段需可被 `AuthService` 访问。`JWTService` 的 `refreshTTL` 为非导出字段，同包 `services` 内可直接访问（`s.jwt.refreshTTL`），合法。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/services/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/services/
git commit -m "feat(services): 添加登录与令牌签发"
```

---

## 任务 9：认证服务 — 刷新（轮换 + 重放检测）

**文件：**
- 修改：`internal/services/auth_service.go`
- 修改：`internal/services/auth_service_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `auth_service_test.go` 追加：
```go
func TestAuthService_Refresh_Success_Rotates(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("alice", "alice@example.com", "Password123")
	r1, _ := svc.Login("alice", "Password123", "ua", "ip")

	r2, err := svc.Refresh(r1.RefreshToken, "ua", "ip")
	if err != nil {
		t.Fatalf("Refresh 错误: %v", err)
	}
	if r2.RefreshToken == r1.RefreshToken {
		t.Error("期望轮换后 refresh token 不同")
	}
	if r2.AccessToken == "" {
		t.Error("期望非空 access token")
	}
}

func TestAuthService_Refresh_OldTokenRevoked(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("bob", "bob@example.com", "Password123")
	r1, _ := svc.Login("bob", "bob@example.com", "Password123", "ua", "ip")

	_, _ = svc.Refresh(r1.RefreshToken, "ua", "ip")
	// 旧 token 再次使用应失败（已吊销）
	if _, err := svc.Refresh(r1.RefreshToken, "ua", "ip"); err != ErrInvalidToken {
		t.Errorf("期望旧 token 失败，实际 %v", err)
	}
}

func TestAuthService_Refresh_ReplayRevokesAll(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("carol", "carol@example.com", "Password123")
	r1, _ := svc.Login("carol", "carol@example.com", "Password123", "ua", "ip")
	r2, _ := svc.Refresh(r1.RefreshToken, "ua", "ip")

	// r1 已吊销；重放 r1 应触发吊销该用户全部令牌
	_, err := svc.Refresh(r1.RefreshToken, "ua", "ip")
	if err != ErrInvalidToken {
		t.Fatalf("期望 ErrInvalidToken，实际 %v", err)
	}
	// 现在 r2 也应失效
	if _, err := svc.Refresh(r2.RefreshToken, "ua", "ip"); err != ErrInvalidToken {
		t.Errorf("重放后 r2 应被吊销，实际 %v", err)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/services/...`
预期：FAIL，`Refresh` 未定义。

- [ ] **步骤 3：编写实现**

在 `auth_service.go` 追加：
```go
func (s *AuthService) Refresh(refreshToken, userAgent, ip string) (*AuthResult, error) {
	claims, err := s.jwt.Parse(refreshToken)
	if err != nil || claims.TokenType != TokenTypeRefresh {
		return nil, ErrInvalidToken
	}

	stored, err := s.tokens.FindByJTI(claims.JTI)
	if err != nil {
		return nil, ErrInvalidToken
	}

	// 重放检测：已吊销的 token 被再次使用 → 吊销该用户全部令牌
	if stored.Revoked {
		_ = s.tokens.RevokeAllByUser(stored.UserID)
		return nil, ErrInvalidToken
	}

	// 校验用户仍存在且未禁用
	user, err := s.users.FindByID(stored.UserID)
	if err != nil {
		return nil, ErrInvalidToken
	}
	if user.Status == models.StatusDisabled {
		return nil, ErrUserDisabled
	}

	// 轮换：吊销旧 token
	if err := s.tokens.Revoke(stored.TokenID); err != nil {
		return nil, err
	}

	return s.issueTokens(user, userAgent, ip)
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/services/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/services/
git commit -m "feat(services): 添加令牌刷新轮换与重放检测"
```

---

## 任务 10：认证服务 — 登出

**文件：**
- 修改：`internal/services/auth_service.go`
- 修改：`internal/services/auth_service_test.go`

- [ ] **步骤 1：编写失败的测试**

在 `auth_service_test.go` 追加：
```go
func TestAuthService_Logout(t *testing.T) {
	svc, _ := newAuthSvc(t)
	_, _ = svc.Register("alice", "alice@example.com", "Password123")
	r, _ := svc.Login("alice", "alice@example.com", "Password123", "ua", "ip")

	if err := svc.Logout(r.RefreshToken); err != nil {
		t.Fatalf("Logout 错误: %v", err)
	}
	// 登出后刷新应失败
	if _, err := svc.Refresh(r.RefreshToken, "ua", "ip"); err != ErrInvalidToken {
		t.Errorf("期望登出后刷新失败，实际 %v", err)
	}
}

func TestAuthService_GetCurrentUser(t *testing.T) {
	svc, _ := newAuthSvc(t)
	u, _ := svc.Register("bob", "bob@example.com", "Password123")

	got, err := svc.GetUserByID(u.ID)
	if err != nil {
		t.Fatalf("GetUserByID 错误: %v", err)
	}
	if got.Username != "bob" {
		t.Errorf("Username = %q", got.Username)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/services/...`
预期：FAIL，`Logout` / `GetUserByID` 未定义。

- [ ] **步骤 3：编写实现**

在 `auth_service.go` 追加：
```go
func (s *AuthService) Logout(refreshToken string) error {
	claims, err := s.jwt.Parse(refreshToken)
	if err != nil || claims.TokenType != TokenTypeRefresh {
		return ErrInvalidToken
	}
	return s.tokens.Revoke(claims.JTI)
}

func (s *AuthService) GetUserByID(id uint) (*models.User, error) {
	return s.users.FindByID(id)
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/services/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/services/
git commit -m "feat(services): 添加登出与当前用户查询"
```

---

## 任务 11：统一响应辅助

**文件：**
- 创建：`internal/handlers/response.go`

- [ ] **步骤 1：编写实现**

创建 `internal/handlers/response.go`：
```go
package handlers

import "github.com/gin-gonic/gin"

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func Success(c *gin.Context, status int, data interface{}) {
	c.JSON(status, gin.H{"data": data})
}

func Fail(c *gin.Context, status int, code, message string) {
	c.JSON(status, gin.H{"error": APIError{Code: code, Message: message}})
}
```

- [ ] **步骤 2：验证编译**

运行：`go build ./internal/handlers/...`
预期：无错误。

- [ ] **步骤 3：Commit**

```bash
git add internal/handlers/
git commit -m "feat(handlers): 添加统一响应辅助"
```

---

## 任务 12：认证 handler（集成测试）

**文件：**
- 创建：`internal/handlers/auth_handler.go`
- 测试：`internal/handlers/auth_handler_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/handlers/auth_handler_test.go`：
```go
package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"opennexus/database"
	"opennexus/services"
)

func setupRouter(t *testing.T) (*gin.Engine, *services.AuthService) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Connect("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("连接测试库失败: %v", err)
	}
	db.Exec("DELETE FROM users")
	db.Exec("DELETE FROM refresh_tokens")
	jwtSvc := services.NewJWTService("this-is-a-very-long-jwt-secret-key-32+bytes!", 15*time.Minute, time.Hour)
	authSvc := services.NewAuthService(db, jwtSvc, 10)
	h := NewAuthHandler(authSvc)

	r := gin.New()
	v1 := r.Group("/api/v1")
	auth := v1.Group("/auth")
	auth.POST("/register", h.Register)
	auth.POST("/login", h.Login)
	auth.POST("/refresh", h.Refresh)
	auth.POST("/logout", h.Logout)
	v1.GET("/me", h.Me) // 未经中间件保护，仅测 handler 内部逻辑
	return r, authSvc
}

func doJSON(t *testing.T, r http.Handler, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_Register_Success(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/auth/register", gin.H{
		"username": "alice", "email": "alice@example.com", "password": "Password123",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("状态码 = %d, 期望 201, body=%s", w.Code, w.Body.String())
	}
}

func TestHandler_Register_WeakPassword(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/auth/register", gin.H{
		"username": "bob", "email": "bob@example.com", "password": "short",
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("状态码 = %d, 期望 400", w.Code)
	}
}

func TestHandler_Register_Duplicate(t *testing.T) {
	r, _ := setupRouter(t)
	_ = doJSON(t, r, "POST", "/api/v1/auth/register", gin.H{"username": "carol", "email": "carol@example.com", "password": "Password123"})
	w := doJSON(t, r, "POST", "/api/v1/auth/register", gin.H{"username": "carol", "email": "other@example.com", "password": "Password123"})
	if w.Code != http.StatusConflict {
		t.Fatalf("状态码 = %d, 期望 409", w.Code)
	}
}

func TestHandler_Login_Success(t *testing.T) {
	r, _ := setupRouter(t)
	_ = doJSON(t, r, "POST", "/api/v1/auth/register", gin.H{"username": "dave", "email": "dave@example.com", "password": "Password123"})

	w := doJSON(t, r, "POST", "/api/v1/auth/login", gin.H{"account": "dave", "password": "Password123"})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		Data struct {
			AccessToken  string `json:"access_token"`
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data.AccessToken == "" || resp.Data.RefreshToken == "" {
		t.Error("期望返回非空令牌")
	}
}

func TestHandler_Login_InvalidCreds(t *testing.T) {
	r, _ := setupRouter(t)
	w := doJSON(t, r, "POST", "/api/v1/auth/login", gin.H{"account": "nobody", "password": "Password123"})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

func TestHandler_Refresh_Success(t *testing.T) {
	r, _ := setupRouter(t)
	_ = doJSON(t, r, "POST", "/api/v1/auth/register", gin.H{"username": "eve", "email": "eve@example.com", "password": "Password123"})
	lw := doJSON(t, r, "POST", "/api/v1/auth/login", gin.H{"account": "eve", "password": "Password123"})

	var login struct {
		Data struct {
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(lw.Body.Bytes(), &login)

	w := doJSON(t, r, "POST", "/api/v1/auth/refresh", gin.H{"refresh_token": login.Data.RefreshToken})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
}

func TestHandler_Logout_Success(t *testing.T) {
	r, _ := setupRouter(t)
	_ = doJSON(t, r, "POST", "/api/v1/auth/register", gin.H{"username": "frank", "email": "frank@example.com", "password": "Password123"})
	lw := doJSON(t, r, "POST", "/api/v1/auth/login", gin.H{"account": "frank", "password": "Password123"})

	var login struct {
		Data struct {
			RefreshToken string `json:"refresh_token"`
		} `json:"data"`
	}
	_ = json.Unmarshal(lw.Body.Bytes(), &login)

	w := doJSON(t, r, "POST", "/api/v1/auth/logout", gin.H{"refresh_token": login.Data.RefreshToken})
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", w.Code)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/handlers/...`
预期：FAIL，编译错误（`NewAuthHandler`、`AuthHandler` 方法未定义）。

- [ ] **步骤 3：编写实现**

创建 `internal/handlers/auth_handler.go`：
```go
package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"opennexus/services"
)

type AuthHandler struct {
	svc *services.AuthService
}

func NewAuthHandler(svc *services.AuthService) *AuthHandler {
	return &AuthHandler{svc: svc}
}

type registerRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	user, err := h.svc.Register(req.Username, req.Email, req.Password)
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	Success(c, http.StatusCreated, user)
}

type loginRequest struct {
	Account  string `json:"account" binding:"required"`
	Password string `json:"password" binding:"required"`
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	result, err := h.svc.Login(req.Account, req.Password, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	Success(c, http.StatusOK, result)
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

func (h *AuthHandler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	result, err := h.svc.Refresh(req.RefreshToken, c.Request.UserAgent(), c.ClientIP())
	if err != nil {
		h.writeAuthError(c, err)
		return
	}
	Success(c, http.StatusOK, result)
}

func (h *AuthHandler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		Fail(c, http.StatusBadRequest, "INVALID_REQUEST", "请求参数无效")
		return
	}
	if err := h.svc.Logout(req.RefreshToken); err != nil {
		h.writeAuthError(c, err)
		return
	}
	Success(c, http.StatusOK, struct{}{})
}

func (h *AuthHandler) Me(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		Fail(c, http.StatusUnauthorized, "UNAUTHORIZED", "未认证")
		return
	}
	user, err := h.svc.GetUserByID(userID.(uint))
	if err != nil {
		Fail(c, http.StatusNotFound, "USER_NOT_FOUND", "用户不存在")
		return
	}
	Success(c, http.StatusOK, user)
}

// writeAuthError 将 service 层错误映射为统一 HTTP 响应
func (h *AuthHandler) writeAuthError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrWeakPassword):
		Fail(c, http.StatusBadRequest, "WEAK_PASSWORD", "密码强度不足（至少 8 位，含字母与数字）")
	case errors.Is(err, services.ErrUserExists):
		Fail(c, http.StatusConflict, "USER_EXISTS", "用户名或邮箱已存在")
	case errors.Is(err, services.ErrInvalidCreds):
		Fail(c, http.StatusUnauthorized, "INVALID_CREDENTIALS", "账号或密码错误")
	case errors.Is(err, services.ErrUserDisabled):
		Fail(c, http.StatusForbidden, "USER_DISABLED", "用户已被禁用")
	case errors.Is(err, services.ErrInvalidToken):
		Fail(c, http.StatusUnauthorized, "INVALID_TOKEN", "无效或已过期的令牌")
	default:
		Fail(c, http.StatusInternalServerError, "INTERNAL", "内部错误")
	}
}
```

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/handlers/...`
预期：PASS。

- [ ] **步骤 5：Commit**

```bash
git add internal/handlers/
git commit -m "feat(handlers): 添加认证 handler 与错误映射"
```

---

## 任务 13：认证中间件（AuthRequired + RequireRole）

**文件：**
- 创建：`internal/middleware/auth_middleware.go`
- 测试：`internal/middleware/auth_middleware_test.go`

- [ ] **步骤 1：编写失败的测试**

创建 `internal/middleware/auth_middleware_test.go`：
```go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"opennexus/services"
)

const mwSecret = "this-is-a-very-long-jwt-secret-key-32+bytes!"

func newEngineWithAuth() (*gin.Engine, *services.JWTService) {
	gin.SetMode(gin.TestMode)
	jwtSvc := services.NewJWTService(mwSecret, 15*time.Minute, time.Hour)
	r := gin.New()
	r.Use(AuthRequired(jwtSvc))
	r.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"user_id": c.GetUint("user_id"), "role": c.GetString("role")})
	})
	return r, jwtSvc
}

func TestAuthRequired_ValidToken(t *testing.T) {
	r, jwtSvc := newEngineWithAuth()
	token, _ := jwtSvc.GenerateAccessToken(5, "alice", "user")

	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200, body=%s", w.Code, w.Body.String())
	}
}

func TestAuthRequired_MissingToken(t *testing.T) {
	r, _ := newEngineWithAuth()
	req := httptest.NewRequest("GET", "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

func TestAuthRequired_InvalidToken(t *testing.T) {
	r, _ := newEngineWithAuth()
	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Authorization", "Bearer not-a-jwt")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("状态码 = %d, 期望 401", w.Code)
	}
}

func TestAuthRequired_RefreshTokenRejected(t *testing.T) {
	r, jwtSvc := newEngineWithAuth()
	token, _, _ := jwtSvc.GenerateRefreshToken(5)
	req := httptest.NewRequest("GET", "/ping", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh token 不应用于访问，期望 401，实际 %d", w.Code)
	}
}

func TestRequireRole_Allowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := services.NewJWTService(mwSecret, 15*time.Minute, time.Hour)
	r := gin.New()
	r.Use(AuthRequired(jwtSvc), RequireRole("admin"))
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

	token, _ := jwtSvc.GenerateAccessToken(1, "admin", "admin")
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("状态码 = %d, 期望 200", w.Code)
	}
}

func TestRequireRole_Forbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	jwtSvc := services.NewJWTService(mwSecret, 15*time.Minute, time.Hour)
	r := gin.New()
	r.Use(AuthRequired(jwtSvc), RequireRole("admin"))
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

	token, _ := jwtSvc.GenerateAccessToken(2, "norm", "user")
	req := httptest.NewRequest("GET", "/x", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("状态码 = %d, 期望 403", w.Code)
	}
}
```

- [ ] **步骤 2：运行测试验证失败**

运行：`go test ./internal/middleware/...`
预期：FAIL，编译错误（`AuthRequired`、`RequireRole` 未定义）。

- [ ] **步骤 3：编写实现**

创建 `internal/middleware/auth_middleware.go`：
```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"opennexus/services"
)

const (
	ctxUserID   = "user_id"
	ctxUsername = "username"
	ctxRole     = "role"
)

func AuthRequired(jwtSvc *services.JWTService) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" || !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "缺少认证令牌"}})
			return
		}
		tokenStr := strings.TrimPrefix(header, "Bearer ")
		claims, err := jwtSvc.Parse(tokenStr)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "无效的令牌"}})
			return
		}
		if claims.TokenType != services.TokenTypeAccess {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "无效的令牌"}})
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxUsername, claims.Username)
		c.Set(ctxRole, claims.Role)
		c.Next()
	}
}

func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role, exists := c.Get(ctxRole)
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "UNAUTHORIZED", "message": "未认证"}})
			return
		}
		if _, ok := allowed[role.(string)]; !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"code": "FORBIDDEN", "message": "权限不足"}})
			return
		}
		c.Next()
	}
}
```

> 注意：`auth_middleware.go` 将上下文 key 用常量管理。`auth_handler.go` 中 `Me` 使用 `c.Get("user_id")`，需与中间件常量一致——在任务 14（router 装配）中将统一通过 `middleware` 包暴露 key 或在 handler 中改用 `middleware.UserIDKey()`。本计划选择在 `middleware` 包导出 `UserIDKey()`：

在 `auth_middleware.go` 末尾追加：
```go
func UserIDKey() string { return ctxUserID }
func RoleKey() string   { return ctxRole }
```

并在 `auth_handler.go` 的 `Me` 中将 `c.Get("user_id")` 改为 `c.Get(middleware.UserIDKey())`（需 import `opennexus/middleware`）。

- [ ] **步骤 4：运行测试验证通过**

运行：`go test ./internal/middleware/...`
预期：PASS。
运行：`go test ./internal/handlers/...`
预期：PASS（handler 与中间件 key 一致）。

- [ ] **步骤 5：Commit**

```bash
git add internal/middleware/ internal/handlers/
git commit -m "feat(middleware): 添加认证与角色中间件"
```

---

## 任务 14：路由装配

**文件：**
- 创建：`internal/router/router.go`

- [ ] **步骤 1：编写实现**

创建 `internal/router/router.go`：
```go
package router

import (
	"github.com/gin-gonic/gin"

	"opennexus/handlers"
	"opennexus/middleware"
	"opennexus/services"
)

func Setup(authSvc *services.AuthService, jwtSvc *services.JWTService, mode string) *gin.Engine {
	gin.SetMode(mode)
	r := gin.New()
	r.Use(gin.Recovery())

	authHandler := handlers.NewAuthHandler(authSvc)

	v1 := r.Group("/api/v1")
	{
		auth := v1.Group("/auth")
		{
			auth.POST("/register", authHandler.Register)
			auth.POST("/login", authHandler.Login)
			auth.POST("/refresh", authHandler.Refresh)
			auth.POST("/logout", authHandler.Logout)
		}

		protected := v1.Group("")
		protected.Use(middleware.AuthRequired(jwtSvc))
		{
			protected.GET("/me", authHandler.Me)
		}
	}

	health := r.Group("/health")
	{
		health.GET("", func(c *gin.Context) {
			c.JSON(200, gin.H{"status": "ok"})
		})
	}

	return r
}
```

- [ ] **步骤 2：验证编译**

运行：`go build ./...`
预期：无错误。

- [ ] **步骤 3：Commit**

```bash
git add internal/router/
git commit -m "feat(router): 装配认证路由"
```

---

## 任务 15：程序入口与启动校验

**文件：**
- 创建：`cmd/server/main.go`

- [ ] **步骤 1：编写实现**

创建 `cmd/server/main.go`：
```go
package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/gin-gonic/gin"

	"opennexus/config"
	"opennexus/database"
	"opennexus/router"
	"opennexus/services"
)

func main() {
	cfgPath := "config.yaml"
	if p := os.Getenv("CONFIG_PATH"); p != "" {
		cfgPath = p
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}
	if err := cfg.Validate(); err != nil {
		log.Fatalf("配置校验失败: %v", err)
	}

	if cfg.Database.Path != ":memory:" {
		if err := os.MkdirAll(filepathOf(cfg.Database.Path), 0o755); err != nil {
			log.Fatalf("创建数据库目录失败: %v", err)
		}
	}

	db, err := database.Connect(cfg.Database.Path)
	if err != nil {
		log.Fatalf("连接数据库失败: %v", err)
	}

	jwtSvc := services.NewJWTService(cfg.JWT.Secret, cfg.JWT.AccessTTL, cfg.JWT.RefreshTTL)
	authSvc := services.NewAuthService(db, jwtSvc, cfg.Password.BcryptCost)

	engine := router.Setup(authSvc, jwtSvc, cfg.Server.Mode)

	go func() {
		addr := fmt.Sprintf(":%d", cfg.Server.Port)
		log.Printf("openNexus 认证服务启动于 %s", addr)
		if err := engine.Run(addr); err != nil {
			log.Fatalf("服务器启动失败: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("服务正在关闭...")
}

// filepathOf 返回数据库文件所在目录
func filepathOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' {
			return p[:i]
		}
	}
	return "."
}
```

> 注意：避免与标准库 `path/filepath` 命名冲突，使用本地 `filepathOf` 函数而非导入。

- [ ] **步骤 2：验证编译**

运行：`go build ./...`
预期：无错误。

- [ ] **步骤 3：全量测试**

运行：`go test ./...`
预期：全部 PASS。

- [ ] **步骤 4：Commit**

```bash
git add cmd/
git commit -m "feat(server): 添加程序入口与启动校验"
```

---

## 任务 16：端到端冒烟测试

**文件：** 无（手动验证）

- [ ] **步骤 1：启动服务**

运行（设置一个合法 secret）：
```bash
JWT_SECRET="this-is-a-very-long-jwt-secret-key-32+bytes!" SERVER_PORT=8080 \
DATABASE_PATH="./data/opennexus.db" go run ./cmd/server
```
预期：日志输出 `openNexus 认证服务启动于 :8080`。

- [ ] **步骤 2：注册**

运行：
```bash
curl -s -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","email":"alice@example.com","password":"Password123"}'
```
预期：HTTP 201，返回含 `data.user`。

- [ ] **步骤 3：登录**

运行：
```bash
curl -s -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"account":"alice","password":"Password123"}'
```
预期：HTTP 200，返回 `access_token` 与 `refresh_token`。记录两者。

- [ ] **步骤 4：访问 /me**

运行（用上一步的 access_token）：
```bash
curl -s http://localhost:8080/api/v1/me -H 'Authorization: Bearer <ACCESS_TOKEN>'
```
预期：HTTP 200，返回 `data.user`（即 alice）。

- [ ] **步骤 5：刷新**

运行（用 refresh_token）：
```bash
curl -s -X POST http://localhost:8080/api/v1/auth/refresh \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<REFRESH_TOKEN>"}'
```
预期：HTTP 200，返回新的令牌对。

- [ ] **步骤 6：登出**

运行（用刷新后的新 refresh_token）：
```bash
curl -s -X POST http://localhost:8080/api/v1/auth/logout \
  -H 'Content-Type: application/json' \
  -d '{"refresh_token":"<NEW_REFRESH_TOKEN>"}'
```
预期：HTTP 200。

- [ ] **步骤 7：健康检查**

运行：`curl -s http://localhost:8080/health`
预期：`{"status":"ok"}`。

- [ ] **步骤 8：Commit（记录可运行状态）**

```bash
git add -A
git commit -m "chore: P3 用户认证系统端到端冒烟通过"
```

---

## 验收对照（成功标准 ↔ 任务）

| 成功标准（规格 §13） | 覆盖任务 |
|----------------------|---------|
| 五个端点可用且通过测试 | 任务 12, 16 |
| Access 15m / Refresh 7d 可吊销 | 任务 6, 5, 8, 10 |
| 刷新轮换 + 重放检测 | 任务 9 |
| 中间件可被后续复用 | 任务 13, 14 |
| 单机 SQLite 启动 + yaml/env 配置 | 任务 2, 3, 15 |
