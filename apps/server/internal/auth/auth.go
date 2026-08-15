package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/rbac"
)

// UserProfile 用户 + 角色 + 权限（/api/me 与登录响应）。
type UserProfile struct {
	User        dsmodel.User `json:"user"`
	Roles       []string     `json:"roles"`
	Permissions []string     `json:"permissions"`
}

// Manager 负责登录、当前用户、改密与中间件。
// 依赖：db（用户查询/改密）、rbac 缓存（权限即时解析）、JWT 密钥。
type Manager struct {
	secret string
	ttl    time.Duration
	db     *gorm.DB
	cache  *rbac.Cache
}

// New 构建认证管理器。
func New(secret string, ttl time.Duration, db *gorm.DB, cache *rbac.Cache) *Manager {
	return &Manager{secret: secret, ttl: ttl, db: db, cache: cache}
}

// ErrUnauthorized 统一认证错误（不向客户端泄露具体原因）。
var ErrUnauthorized = errors.New("用户名或密码错误")

// ErrNotFoundUser 用户不存在。
var ErrNotFoundUser = errors.New("user not found")

// ErrBadOldPassword 旧密码错误。
var ErrBadOldPassword = errors.New("旧密码不正确")

// ErrInvalidToken token 非法/过期/版本不符。
var ErrInvalidToken = errors.New("invalid token")

// HashPassword 生成 bcrypt 哈希。
func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

// Login 校验用户名密码，签发 token，返回 token + 用户 + 角色 + 权限并集。
func (m *Manager) Login(ctx context.Context, username, password string) (token string, userID uint64, roles, perms []string, err error) {
	username = strings.TrimSpace(username)
	var u struct {
		ID           uint64
		PasswordHash string
		TokenVersion uint64
	}
	if e := m.db.WithContext(ctx).Table("users").Select("id, password_hash, token_version").
		Where("username = ?", username).Scan(&u).Error; e != nil {
		return "", 0, nil, nil, ErrUnauthorized
	}
	if u.ID == 0 {
		return "", 0, nil, nil, ErrUnauthorized
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", 0, nil, nil, ErrUnauthorized
	}
	t, e := IssueToken(m.secret, u.ID, u.TokenVersion, m.ttl)
	if e != nil {
		return "", 0, nil, nil, e
	}
	return t, u.ID, m.cache.UserRoles(u.ID), m.cache.UserPermissions(u.ID), nil
}

// Authenticate 校验 token（签名/过期/token_version），返回 userID。
func (m *Manager) Authenticate(token string) (uint64, error) {
	uid, ver, err := ParseToken(m.secret, token)
	if err != nil {
		return 0, ErrInvalidToken
	}
	// token 版本校验：改密/重置后版本递增，旧 token 立即失效（Q29）。
	cur, ok := m.cache.TokenVersion(uid)
	if !ok || cur != ver {
		return 0, ErrInvalidToken
	}
	return uid, nil
}

// Me 返回当前用户资料 + 角色 + 权限并集（供 /api/me 与前端权限驱动 UI）。
func (m *Manager) Me(ctx context.Context, userID uint64) (*UserProfile, error) {
	var u dsmodel.User
	if e := m.db.WithContext(ctx).First(&u, userID).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return nil, ErrNotFoundUser
		}
		return nil, e
	}
	return &UserProfile{
		User:        u,
		Roles:       m.cache.UserRoles(userID),
		Permissions: m.cache.UserPermissions(userID),
	}, nil
}

// Cache 暴露 RBAC 缓存（供 handler 组装 /api/me 等）。
func (m *Manager) Cache() *rbac.Cache { return m.cache }

// HasPermission 判断用户是否持有权限（内存读）。
func (m *Manager) HasPermission(userID uint64, perm string) bool {
	return m.cache.HasPermission(userID, perm)
}

// ReloadUser 重载单用户 RBAC（用户角色变更/改密后调用）。
func (m *Manager) ReloadUser(userID uint64) error { return m.cache.ReloadUser(m.db, userID) }

// ReloadAll 全量重载 RBAC（角色/权限组变更后调用）。
func (m *Manager) ReloadAll() error { return m.cache.ReloadAll(m.db) }

// ValidPerm 判断权限名是否在目录内（用于输入校验）。
func ValidPerm(p string) bool {
	for _, v := range dsmodel.AllPermissions {
		if v == p {
			return true
		}
	}
	return false
}

// ChangePassword 用户自助改密：验旧密码，落新哈希并 bump token_version，重载缓存。
func (m *Manager) ChangePassword(ctx context.Context, userID uint64, oldPass, newPass string) error {
	if newPass == "" {
		return errors.New("新密码不能为空")
	}
	var u struct {
		ID           uint64
		PasswordHash string
	}
	if e := m.db.WithContext(ctx).Table("users").Select("id, password_hash").First(&u, userID).Error; e != nil {
		return ErrNotFoundUser
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPass)) != nil {
		return ErrBadOldPassword
	}
	hash, err := HashPassword(newPass)
	if err != nil {
		return err
	}
	if e := m.db.WithContext(ctx).Table("users").Where("id = ?", userID).
		UpdateColumn("password_hash", hash).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error; e != nil {
		return e
	}
	return m.cache.ReloadUser(m.db, userID)
}
