package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ha-webui-sdkhandler/internal/config"
)

// newInternalKeyTestRouter 构造一个只含受保护测试路由的 gin 引擎。
// 路由后端返回 200 {"success":true}，用于区分"鉴权通过"与"被拦截"。
func newInternalKeyTestRouter(internalKey string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	a := &app{cfg: &config.Config{}}
	a.cfg.HRPAuth.InternalKey = internalKey

	r := gin.New()
	r.GET("/api/build/status", a.internalKeyAuthMiddleware(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"success": true})
	})
	return r
}

func doInternalKeyRequest(t *testing.T, r *gin.Engine, header string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/build/status", nil)
	if header != "" {
		req.Header.Set("X-Internal-Key", header)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 情形 1：未携带 X-Internal-Key → 401。
func TestInternalKeyAuth_MissingHeader(t *testing.T) {
	r := newInternalKeyTestRouter("test-internal-key")
	w := doInternalKeyRequest(t, r, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without header, got %d body=%s", w.Code, w.Body.String())
	}
}

// 情形 2：携带错误密钥 → 401（组织惯例：无效 Internal Key 返回 401 invalid_credentials）。
func TestInternalKeyAuth_WrongKey(t *testing.T) {
	r := newInternalKeyTestRouter("test-internal-key")
	w := doInternalKeyRequest(t, r, "wrong-key")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 with wrong key, got %d body=%s", w.Code, w.Body.String())
	}
}

// 情形 3：携带正确密钥 → 放行到后端（200）。
func TestInternalKeyAuth_CorrectKey(t *testing.T) {
	r := newInternalKeyTestRouter("test-internal-key")
	w := doInternalKeyRequest(t, r, "test-internal-key")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with correct key, got %d body=%s", w.Code, w.Body.String())
	}
}

// 情形 4：密钥未配置 → fail-closed，任何请求（即使带头）都拒绝。
func TestInternalKeyAuth_UnconfiguredFailClosed(t *testing.T) {
	r := newInternalKeyTestRouter("")
	w := doInternalKeyRequest(t, r, "any-key")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when key unconfigured (fail-closed), got %d body=%s", w.Code, w.Body.String())
	}
}
