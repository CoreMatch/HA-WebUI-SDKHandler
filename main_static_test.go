package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ha-webui-sdkhandler/internal/config"
)

// newStaticTestEnv 构造临时产物目录（含 index.html 与静态文件）以及指向它的
// app，返回与生产相同方式注册路由的引擎。
func newStaticTestEnv(t *testing.T) *gin.Engine {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>index</html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "app.js"), []byte("console.log(1);"), 0o644); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	a := &app{cfg: &config.Config{}}
	a.cfg.Build.CurrentLink = dir

	r := gin.New()
	r.GET("/", a.handleStaticIndex)
	r.NoRoute(a.handleStatic)
	return r
}

func doStaticRequest(t *testing.T, r *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// 根路径返回 index.html。
func TestStatic_IndexServed(t *testing.T) {
	r := newStaticTestEnv(t)
	w := doStaticRequest(t, r, "/")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<html>index</html>") {
		t.Fatalf("expected index.html, got %d body=%q", w.Code, w.Body.String())
	}
}

// 命中的真实静态文件直接返回，不回落。
func TestStatic_RealFileServed(t *testing.T) {
	r := newStaticTestEnv(t)
	w := doStaticRequest(t, r, "/assets/app.js")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "console.log(1);") {
		t.Fatalf("expected app.js content, got %d body=%q", w.Code, w.Body.String())
	}
}

// 未命中的非 /api 路径回落 index.html（SPA 前端路由）。
func TestStatic_SPAFallback(t *testing.T) {
	r := newStaticTestEnv(t)
	w := doStaticRequest(t, r, "/some/spa/route")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "<html>index</html>") {
		t.Fatalf("expected SPA fallback to index.html, got %d body=%q", w.Code, w.Body.String())
	}
}

// 未注册的 /api 路径返回 JSON 404，不回落到前端。
func TestStatic_UnknownAPIPathNoFallback(t *testing.T) {
	r := newStaticTestEnv(t)
	w := doStaticRequest(t, r, "/api/definitely-not-registered")
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "route not found") {
		t.Fatalf("expected JSON 404 for unknown /api path, got %d body=%q", w.Code, w.Body.String())
	}
}

// 越界路径被拒绝（纵深防御；net/http 之外再挡一层）。
// 编码形式（%2f）的遍历请求由 gin 以 "invalid URL path" 直接 400，
// 这里用未编码形式验证本处理器的守卫。
func TestStatic_TraversalRejected(t *testing.T) {
	r := newStaticTestEnv(t)
	w := doStaticRequest(t, r, "/../secret")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "非法路径") {
		t.Fatalf("expected 400 for traversal path, got %d body=%q", w.Code, w.Body.String())
	}
}

// 无已发布产物时：根路径与任意路径均为 JSON 404。
func TestStatic_NoPublishedArtifacts(t *testing.T) {
	gin.SetMode(gin.TestMode)
	a := &app{cfg: &config.Config{}}
	a.cfg.Build.CurrentLink = filepath.Join(os.TempDir(), "sdkhandler-static-test-missing-dir")

	r := gin.New()
	r.GET("/", a.handleStaticIndex)
	r.NoRoute(a.handleStatic)

	for _, path := range []string{"/", "/foo"} {
		w := doStaticRequest(t, r, path)
		if w.Code != http.StatusNotFound {
			t.Fatalf("path %s: expected 404 without published artifacts, got %d", path, w.Code)
		}
	}
}
