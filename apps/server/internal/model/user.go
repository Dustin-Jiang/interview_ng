package model

import "time"

// User 面试官（登录用户）。
type User struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128" json:"name"`
	Role      string    `gorm:"size:32" json:"role"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
