package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"nexusagent/internal/services"
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
