package handler

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

/*
RegisterStatic —— 由后端进程直接提供前端构建产物（单进程部署，无需额外 web 服务器）。

命中磁盘文件就发文件，未命中回落 index.html：前端是 history 路由，
/candidates/:id、/settings/* 这类深链刷新不能 404。

只挂在 gin 的 NoRoute 上（路由表未匹配时才触发），因此 /api、/ws 的既有行为不受影响；
反过来，未匹配的 /api、/ws 必须保持 404、不能回落成前端页面——否则接口写错路径会拿到
200 + HTML，比 404 更难排查。

root 为空、目录不存在或没有 index.html 时返回 false 且不注册任何东西：本地开发仍由
Vite（:3000）提供前端，后端只跑 API/WS，行为与加这段之前完全一致。
*/
func RegisterStatic(r *gin.Engine, root string) bool {
	if root == "" {
		return false
	}
	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return false
	}
	index := filepath.Join(root, "index.html")
	if fi, err := os.Stat(index); err != nil || !fi.Mode().IsRegular() {
		return false
	}

	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/ws/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not_found"})
			return
		}
		// 只有 GET/HEAD 走前端回退；其余方法在未知路径上一律 404
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Status(http.StatusNotFound)
			return
		}

		// 归一化成 root 下的相对路径：先 Clean 掉 "." / ".."，避免请求穿越出 root
		rel := strings.TrimPrefix(path.Clean("/"+p), "/")
		if rel != "" {
			full := filepath.Join(root, filepath.FromSlash(rel))
			if fi, err := os.Stat(full); err == nil && fi.Mode().IsRegular() {
				// 带内容哈希的产物可长缓存；其余（favicon 等）保持不缓存
				if strings.HasPrefix(rel, "assets/") {
					c.Header("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					c.Header("Cache-Control", "no-cache")
				}
				c.File(full)
				return
			}
		}

		// 未命中 → 入口文件。入口文件不缓存，否则升级后仍会引用已删除的旧 chunk
		c.Header("Cache-Control", "no-cache")
		c.File(index)
	})
	return true
}
