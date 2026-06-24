# NexusAgent 用户认证系统（P3）设计文档

- 日期：2026-06-24
- 子项目：P3 — 用户认证系统
- 状态：待审查

## 1. 背景与定位

NexusAgent 是一个用 Go 开发的全栈平台，目标是通过 ACP（Agent Client Protocol）协议调用 Claude Code、Codex 等编码 agent 执行任务，并支持用户认证。

整个平台被拆分为 6 个可独立设计的子项目：

| # | 子项目 | 依赖 |
|----|--------|------|
| P1 | ACP 客户端核心库 | 无 |
| P2 | Agent 注册与编排层 | P1 |
| P3 | 用户认证系统（本文档） | 无 |
| P4 | 任务/会话管理 + 工作区 | P1, P2 |
| P5 | REST API 接入层 | P2, P3, P4 |
| P6 | Web UI | P5 |

本文档仅覆盖 **P3 用户认证系统**。P3 不依赖其他子项目，可独立先行实现。P3 选定的技术栈（Gin + GORM）将作为整个平台的基石，延续到后续子项目。

## 2. 需求摘要

- 用户可注册账号（用户名 + 邮箱 + 密码）。
- 用户可登录（用户名或邮箱 + 密码），获得访问令牌。
- 令牌可刷新、可登出、可吊销。
- 已认证的 API 端点能识别当前用户并按角色做权限控制（RBAC 预留）。
- 单机部署，SQLite 存储，后续可平滑迁移到 PostgreSQL。

## 3. 技术栈

| 组件 | 选型 |
|------|------|
| 语言 | Go（最新稳定版） |
| Web 框架 | Gin |
| ORM | GORM |
| 数据库 | SQLite（单机起步） |
| 密码哈希 | bcrypt（cost=12，可配置） |
| JWT | golang-jwt，HS256 |
| 令牌模型 | Access Token（短期，无状态）+ Refresh Token（长期，落库可吊销） |

## 4. 架构与目录结构

分层架构，各层职责清晰，便于后续子项目复用模式：

```
cmd/server/main.go              # 程序入口
internal/
  config/                       # 配置加载（yaml + 环境变量覆盖）
  database/                     # SQLite 连接、GORM 自动迁移
  models/                       # GORM 模型（user, refresh_token）
  repository/                   # 数据访问层（CRUD，隔离 GORM）
  services/
    auth_service.go             # 注册/登录/刷新/登出 业务逻辑
    jwt_service.go              # JWT 签发/校验
  handlers/
    auth_handler.go             # Gin HTTP handler
  middleware/
    auth_middleware.go          # JWT 校验、用户注入、RBAC
  router/
    router.go                   # 路由注册
pkg/                            # 可复用工具（未来跨子项目共享）
go.mod
```

职责边界：

- `repository` 只做数据库读写，不含业务规则。
- `services` 包含业务逻辑，不感知 HTTP。
- `handlers` 只做请求解析与响应组装，调用 service。
- `middleware` 做认证拦截与权限校验。

## 5. 数据模型

### 5.1 `users` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | uint | PK，自增 | 主键 |
| `username` | string | unique，索引 | 登录名 |
| `email` | string | unique，索引 | 邮箱 |
| `password_hash` | string | 非空 | bcrypt 哈希 |
| `role` | string | 非空，默认 `user` | 枚举：`admin` / `user` |
| `status` | string | 非空，默认 `active` | 枚举：`active` / `disabled` |
| `created_at` | time | | 创建时间 |
| `updated_at` | time | | 更新时间 |
| `deleted_at` | time | 索引 | 软删除时间 |

### 5.2 `refresh_tokens` 表

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `id` | uint | PK，自增 | 主键 |
| `user_id` | uint | FK，索引 | 所属用户 |
| `token_id` | string | unique，索引 | JWT 的 `jti` 声明 |
| `expires_at` | time | 非空 | 过期时间 |
| `revoked` | bool | 默认 false | 是否已吊销 |
| `user_agent` | string | | 签发时的客户端 UA（审计） |
| `ip` | string | | 签发时的 IP（审计） |
| `created_at` | time | | 创建时间 |

设计要点：

- 密码永不存明文，仅 bcrypt 哈希。
- Refresh Token 用 `jti` 唯一标识并落库，从而可吊销。
- 用户使用软删除，保留审计痕迹。

## 6. 令牌机制

### 6.1 Access Token

- 算法：HS256，密钥来自配置 `JWT_SECRET`。
- 声明：`sub`（用户 ID）、`role`、`username`、`jti`（UUID）、`exp`、`iat`。
- 有效期：15 分钟（可配置 `ACCESS_TTL`）。
- 校验：验签 + 校验 `exp`；无状态，不查库。
- 传递：HTTP 头 `Authorization: Bearer <token>`。

### 6.2 Refresh Token

- 算法：HS256，与 Access Token 同密钥。
- 声明：`sub`（用户 ID）、`jti`（UUID）、`exp`、`iat`，不含敏感信息。
- 有效期：7 天（可配置 `REFRESH_TTL`）。
- 校验：验签 + 校验 `exp` + 查库确认 `jti` 存在、`revoked=false`。
- 传递：放请求体。

### 6.3 登录流程

1. 校验账号（用户名或邮箱）与密码。
2. 签发 Access Token 与 Refresh Token。
3. 将 Refresh Token 的 `jti` 落库，记录 UA/IP。
4. 返回 `access_token`、`refresh_token`、`expires_in`、`user`。

### 6.4 刷新流程（轮换）

1. 校验 Refresh Token 合法且未吊销、未过期。
2. 将旧记录 `revoked=true`。
3. 签发新的 Access Token 与 Refresh Token。
4. 将新 `jti` 落库。
5. 返回新令牌对。
6. 重放检测：若检测到已吊销的 Refresh Token 被使用，吊销该用户的全部 Refresh Token。

### 6.5 登出流程

1. 接收 Refresh Token。
2. 将对应 `jti` 标记 `revoked=true`。
3. 返回成功。

## 7. API 端点设计

统一响应格式：

- 成功：`{ "data": ... }`
- 失败：`{ "error": { "code": "...", "message": "..." } }`

### 7.1 认证端点（前缀 `/api/v1/auth`，无需登录）

| 方法 | 路径 | 说明 | 请求体 | 成功响应 |
|------|------|------|--------|---------|
| POST | `/register` | 注册 | `{username, email, password}` | 201，`{user}` |
| POST | `/login` | 登录 | `{account, password}`（account 为用户名或邮箱） | 200，`{access_token, refresh_token, expires_in, user}` |
| POST | `/refresh` | 刷新令牌 | `{refresh_token}` | 200，`{access_token, refresh_token, expires_in}` |
| POST | `/logout` | 登出 | `{refresh_token}` | 200，`{}` |

### 7.2 当前用户端点（前缀 `/api/v1`，需登录）

| 方法 | 路径 | 说明 | 成功响应 |
|------|------|------|---------|
| GET | `/me` | 获取当前登录用户信息 | 200，`{user}` |

`/me` 同时作为已认证端点的样板，为后续 P4/P5 业务端点示范中间件用法。

### 7.3 错误码约定（全局适用，节选）

| 场景 | HTTP | code |
|------|------|------|
| 注册用户名/邮箱已存在 | 409 | `USER_EXISTS` |
| 注册密码不合规 | 400 | `WEAK_PASSWORD` |
| 登录账号或密码错误 | 401 | `INVALID_CREDENTIALS` |
| Refresh Token 无效/过期/已吊销 | 401 | `INVALID_TOKEN` |
| 未携带/无效 Access Token | 401 | `UNAUTHORIZED` |
| 角色权限不足 | 403 | `FORBIDDEN` |
| 用户被禁用 | 403 | `USER_DISABLED` |

## 8. 中间件与 RBAC

### 8.1 `AuthRequired()`

1. 从 `Authorization: Bearer` 提取 Access Token；缺失返回 401 `UNAUTHORIZED`。
2. 校验签名与有效期；失败返回 401 `UNAUTHORIZED`。
3. 将用户信息（ID/role/username）注入 `gin.Context`。
4. 可选：惰性清理过期 refresh token。

### 8.2 `RequireRole(roles ...string)`

- 依赖 `AuthRequired` 已注入的 role。
- role 不在允许列表内返回 403 `FORBIDDEN`。
- 用法示例：`router.Use(middleware.AuthRequired(), middleware.RequireRole("admin"))`。

后续 P4 业务端点直接复用这两个中间件。

## 9. 配置

`config.yaml` + 环境变量覆盖（环境变量优先级更高）：

```yaml
server:
  port: 8080
  mode: debug          # debug | release
database:
  path: ./data/nexus.db
jwt:
  secret: ""           # 由环境变量 JWT_SECRET 注入
  access_ttl: 15m
  refresh_ttl: 168h    # 7 天
password:
  bcrypt_cost: 12
```

## 10. 安全要点

- 密码强度：≥8 位，需同时包含字母与数字。
- 用户名/邮箱唯一性由 DB 唯一约束保证，并返回友好错误。
- 登录失败统一返回"账号或密码错误"，避免用户枚举。
- 启动时校验 `JWT_SECRET` 非空且长度 ≥32 字节，否则拒绝启动。
- 被禁用（`disabled`）用户登录与刷新均被拒绝。
- 预留 CORS 中间件（后续 Web UI 跨域使用）。

## 11. 测试策略

- repository / services 层：使用内存 SQLite 编写单元测试，覆盖注册重复、登录、刷新轮换、吊销、过期、重放检测等。
- handlers 层：使用 `httptest` + Gin 测试模式编写集成测试，校验 HTTP 状态码、响应体与错误码。
- 覆盖目标：核心认证流程 100% 覆盖。

## 12. 范围边界（不做）

- 不实现外部 SSO/OIDC（OAuth 接入留待未来扩展）。
- 不实现多因素认证（MFA）。
- 不实现邮箱验证/找回密码流程。
- 不实现用户管理后台 UI。
- 不涉及 P1/P2/P4/P5/P6 的任何功能。

## 13. 成功标准

- 注册、登录、刷新、登出、获取当前用户五个端点全部可用且通过测试。
- Access Token 15 分钟过期，Refresh Token 7 天过期且可吊销。
- 刷新采用轮换机制，并具备重放检测（吊销全部）。
- `AuthRequired` 与 `RequireRole` 中间件可被后续子项目直接复用。
- 单机 SQLite 可启动运行，配置通过 yaml + 环境变量加载。
