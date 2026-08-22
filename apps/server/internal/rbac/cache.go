package rbac

import (
	"sync"

	"gorm.io/gorm"

	dsmodel "interview_ng/internal/model"
)

// Cache 是 RBAC 权限的内存缓存（Q10=A）：
// 启动全量加载；角色分配/权限组/改密变更时同步失效更新，保证权限变更即时生效。
// 数据权威在库，缓存只加速判断，不做持久化。
type Cache struct {
	mu          sync.RWMutex
	permByUser  map[uint64]map[string]struct{} // userID -> 权限并集
	rolesByUser map[uint64][]dsmodel.Role      // userID -> 角色列表
	tokenVer    map[uint64]uint64              // userID -> token_version
	roleByID    map[uint64]string              // roleID -> roleName
	permByRole  map[uint64]map[string]struct{} // roleID -> 权限集合
}

// New 构建空缓存。
func New() *Cache {
	return &Cache{
		permByUser:  make(map[uint64]map[string]struct{}),
		rolesByUser: make(map[uint64][]dsmodel.Role),
		tokenVer:    make(map[uint64]uint64),
		roleByID:    make(map[uint64]string),
		permByRole:  make(map[uint64]map[string]struct{}),
	}
}

// ReloadAll 从库全量重建缓存（启动与角色/权限组结构变更后调用）。
func (c *Cache) ReloadAll(db *gorm.DB) error {
	var roles []dsmodel.Role
	if err := db.Preload("Permissions").Find(&roles).Error; err != nil {
		return err
	}
	var users []dsmodel.User
	if err := db.Select("id", "token_version").Find(&users).Error; err != nil {
		return err
	}
	var userRoles []dsmodel.UserRole
	if err := db.Find(&userRoles).Error; err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.roleByID = make(map[uint64]string)
	c.permByRole = make(map[uint64]map[string]struct{})
	for _, r := range roles {
		c.roleByID[r.ID] = r.Name
		ps := make(map[string]struct{}, len(r.Permissions))
		for _, p := range r.Permissions {
			ps[p.Permission] = struct{}{}
		}
		c.permByRole[r.ID] = ps
	}

	c.tokenVer = make(map[uint64]uint64, len(users))
	for _, u := range users {
		c.tokenVer[u.ID] = u.TokenVersion
	}

	// 用户 → 角色 + 权限并集
	c.rolesByUser = make(map[uint64][]dsmodel.Role)
	c.permByUser = make(map[uint64]map[string]struct{})
	for _, ur := range userRoles {
		c.rolesByUser[ur.UserID] = append(c.rolesByUser[ur.UserID], dsmodel.Role{ID: ur.RoleID, Name: c.roleByID[ur.RoleID]})
		if c.permByUser[ur.UserID] == nil {
			c.permByUser[ur.UserID] = make(map[string]struct{})
		}
		for p := range c.permByRole[ur.RoleID] {
			c.permByUser[ur.UserID][p] = struct{}{}
		}
	}
	return nil
}

// ReloadUser 重载单个用户的角色/权限（用户角色变更、改密后调用）。
func (c *Cache) ReloadUser(db *gorm.DB, userID uint64) error {
	var u dsmodel.User
	if err := db.First(&u, userID).Error; err != nil {
		return err
	}
	var urs []dsmodel.UserRole
	if err := db.Where("user_id = ?", userID).Find(&urs).Error; err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokenVer[userID] = u.TokenVersion
	c.rolesByUser[userID] = nil
	c.permByUser[userID] = make(map[string]struct{})
	for _, ur := range urs {
		c.rolesByUser[userID] = append(c.rolesByUser[userID], dsmodel.Role{ID: ur.RoleID, Name: c.roleByID[ur.RoleID]})
		for p := range c.permByRole[ur.RoleID] {
			c.permByUser[userID][p] = struct{}{}
		}
	}
	return nil
}

// HasPermission 判断用户是否持有权限（内存读，无 DB 参与）。
func (c *Cache) HasPermission(userID uint64, perm string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ps, ok := c.permByUser[userID]
	if !ok {
		return false
	}
	_, ok = ps[perm]
	return ok
}

// UserRoles 返回用户的角色名列表。
func (c *Cache) UserRoles(userID uint64) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	rs := c.rolesByUser[userID]
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

// UserPermissions 返回用户的权限名列表（并集）。
func (c *Cache) UserPermissions(userID uint64) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ps := c.permByUser[userID]
	out := make([]string, 0, len(ps))
	for p := range ps {
		out = append(out, p)
	}
	return out
}

// TokenVersion 返回用户当前 token 版本；用户不存在返回 (0,false)。
func (c *Cache) TokenVersion(userID uint64) (uint64, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.tokenVer[userID]
	return v, ok
}
