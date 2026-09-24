package handler_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"interview_ng/internal/handler"
)

// 前端产物由后端直接提供时的对外契约：深链回退、缓存头、不遮蔽 /api、不穿越目录。
func TestRegisterStatic(t *testing.T) {
	gin.SetMode(gin.TestMode)

	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("index.html", "<html>app shell</html>")
	write("assets/app-abc123.js", "console.log(1)")

	// 与 root 同级的文件：用于验证目录穿越拿不到它
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	if err := os.WriteFile(secret, []byte("TOPSECRET"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(secret) })

	r := gin.New()
	r.GET("/api/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"pong": true}) })
	if !handler.RegisterStatic(r, root) {
		t.Fatal("RegisterStatic 应当注册成功")
	}

	get := func(path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w
	}

	t.Run("深链未命中磁盘 → 回 index.html 且不缓存", func(t *testing.T) {
		w := get("/candidates/42")
		if w.Code != http.StatusOK {
			t.Fatalf("code=%d", w.Code)
		}
		if got := w.Body.String(); got != "<html>app shell</html>" {
			t.Fatalf("body=%q", got)
		}
		if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Fatalf("Cache-Control=%q", cc)
		}
	})

	t.Run("哈希产物 → 长缓存 + 正确 Content-Type", func(t *testing.T) {
		w := get("/assets/app-abc123.js")
		if w.Code != http.StatusOK || w.Body.String() != "console.log(1)" {
			t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
		}
		if cc := w.Header().Get("Cache-Control"); cc != "public, max-age=31536000, immutable" {
			t.Fatalf("Cache-Control=%q", cc)
		}
		if ct := w.Header().Get("Content-Type"); ct == "" {
			t.Fatal("Content-Type 未设置")
		}
	})

	t.Run("已注册的 /api 路由不受影响", func(t *testing.T) {
		w := get("/api/ping")
		if w.Code != http.StatusOK || w.Body.String() != `{"pong":true}` {
			t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
		}
	})

	t.Run("未注册的 /api 路径仍是 404，不回落成前端页面", func(t *testing.T) {
		w := get("/api/nope")
		if w.Code != http.StatusNotFound {
			t.Fatalf("code=%d body=%q", w.Code, w.Body.String())
		}
		if w.Body.String() == "<html>app shell</html>" {
			t.Fatal("接口路径被前端回退接管了")
		}
	})

	t.Run("非 GET/HEAD 不回落", func(t *testing.T) {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/candidates/42", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("code=%d", w.Code)
		}
	})

	t.Run("目录穿越拿不到 root 之外的文件", func(t *testing.T) {
		w := get("/../../secret.txt")
		if strings.Contains(w.Body.String(), "TOPSECRET") {
			t.Fatalf("穿越读到 root 之外的文件：code=%d body=%q", w.Code, w.Body.String())
		}
		// 具体状态码不作为契约：net/http.ServeFile 对含 ".." 的 URL 路径自己就回 400
		// （我们这层另有一次 path.Clean 归一）。契约只有一条——拿不到 root 之外的内容。
	})
}

// root 不可用时（未设置 / 不存在 / 缺 index.html）不应注册任何东西：调用方据此判断是否打印日志。
func TestRegisterStaticSkipsUnusableRoot(t *testing.T) {
	gin.SetMode(gin.TestMode)

	noIndex := t.TempDir()
	if err := os.WriteFile(filepath.Join(noIndex, "app.js"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, root := range map[string]string{
		"未设置":      "",
		"目录不存在":    filepath.Join(t.TempDir(), "missing"),
		"没有 index": noIndex,
	} {
		r := gin.New()
		if handler.RegisterStatic(r, root) {
			t.Fatalf("%s：不应注册", name)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/anything", nil))
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s：code=%d", name, w.Code)
		}
	}
}
