package model

import "time"

// User 面试官（登录用户）。
type User struct {
	ID           uint64    `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"size:64;uniqueIndex" json:"username"` // 登录凭证，不可改
	PasswordHash string    `gorm:"size:255" json:"-"`
	Name         string    `gorm:"size:128" json:"name"`
	TokenVersion uint64    `json:"-"` // 改密/重置时递增，使旧 token 即时失效
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	Roles        []Role    `gorm:"-" json:"roles,omitempty"` // 由 store 手动填充（user_roles M2M）
}
