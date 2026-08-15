package auth

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// ContextUserKey 存放已认证 userID 的 gin 上下文键。
const ContextUserKey = "auth.user_id"

// userIDFromContext 取中间件写入的 userID。
func userIDFromContext(c *gin.Context) (uint64, bool) {
	v, ok := c.Get(ContextUserKey)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint64)
	return id, ok
}

// RequireAuth 认证中间件：校验 Bearer token + token_version。
// 无 token/非法/过期/版本不符 → 401。
func (m *Manager) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := TokenFromBearer(c.GetHeader("Authorization"))
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "未登录"})
			return
		}
		uid, err := m.Authenticate(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "登录已失效，请重新登录"})
			return
		}
		c.Set(ContextUserKey, uid)
		c.Next()
	}
}

// RequirePerm 权限中间件（须在 RequireAuth 之后）：缺权限 → 403。
func (m *Manager) RequirePerm(perm string) gin.HandlerFunc {
	return func(c *gin.Context) {
		uid, ok := userIDFromContext(c)
		if !ok || !m.cache.HasPermission(uid, perm) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "无操作权限"})
			return
		}
		c.Next()
	}
}

// UserID 供 handler 取当前登录用户 id。
func UserID(c *gin.Context) uint64 {
	id, _ := userIDFromContext(c)
	return id
}
