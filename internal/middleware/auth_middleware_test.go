package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"opennexus/internal/services"
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
