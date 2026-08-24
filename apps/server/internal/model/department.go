package model

import "time"

// Department 部门 —— 面试官（登录用户）所属的组织单元。
type Department struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:64;uniqueIndex" json:"name"`
	Description string    `gorm:"size:255" json:"description"`
	MemberCount int64     `gorm:"-" json:"member_count"` // 该部门下面试官数（ListDepartments 计算填充）
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
