package model

import (
	"strings"
	"time"
)

// OidcConfig 是单点登录（OIDC）配置单行记录（ID 恒为 1，懒初始化）。
type OidcConfig struct {
	ID            uint64    `gorm:"primaryKey" json:"id"`
	Enabled       bool      `gorm:"not null;default:false" json:"enabled"`
	Issuer        string    `gorm:"size:255;not null;default:''" json:"issuer"`
	ClientID      string    `gorm:"size:255;not null;default:''" json:"client_id"`
	ClientSecret  string    `gorm:"size:255;not null;default:''" json:"-"` // 明文永不下发
	Scopes        string    `gorm:"size:255;not null;default:''" json:"scopes"`
	RedirectURL   string    `gorm:"size:255;not null;default:''" json:"redirect_url"`
	AutoProvision bool      `gorm:"not null;default:true" json:"auto_provision"`
	UpdatedAt     time.Time `json:"updated_at"`
	// 两类规则均由 store 填充（按 position 升序），不落配置表。
	RoleRules       []OidcRoleRule `gorm:"-" json:"role_rules"`
	DepartmentRules []OidcDeptRule `gorm:"-" json:"department_rules"`
}

// OidcRoleRule 是一条「声明 → 角色」映射规则：对 ID token 声明求值 JMESPath 表达式，
// 命中即赋予 RoleID（自上而下首个命中生效，Position 从 0 起）。
type OidcRoleRule struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	Position   int    `gorm:"not null;index" json:"position"`
	Expression string `gorm:"size:1024;not null" json:"expression"`
	RoleID     uint64 `gorm:"not null;index" json:"role_id"`
}

// OidcDeptRule 是一条「声明 → 部门」映射规则：命中即把账号所属部门设为 DepartmentID
// （自上而下首个命中生效，Position 从 0 起）。
// 与角色规则的区别：未命中**不拒绝登录**，只是不改变账号现有部门（部门不是权限）。
type OidcDeptRule struct {
	ID           uint64 `gorm:"primaryKey" json:"id"`
	Position     int    `gorm:"not null;index" json:"position"`
	Expression   string `gorm:"size:1024;not null" json:"expression"`
	DepartmentID uint64 `gorm:"not null;index" json:"department_id"`
}

// DefaultOidcScopes 是新建配置时的默认 scope（必须包含 openid）。
const DefaultOidcScopes = "openid profile email"

// ParseScopes 把空格/逗号分隔的 scope 串切成去空切片。
func ParseScopes(s string) []string {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\t' || r == '\n'
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}
