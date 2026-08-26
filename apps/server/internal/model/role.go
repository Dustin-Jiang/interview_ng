package model

import "time"

// Permission 权限名常量 —— RBAC 目录定稿（9 枚）。
const (
	PermUsersManage         = "users.manage"          // 面试官/角色管理
	PermCandidatesManage    = "candidates.manage"     // 编辑/删除候选人、重置状态
	PermCandidatesBrowseAll = "candidates.browse_all" // 跨部门浏览候选人录取状态
	PermCandidatesCreate    = "candidates.create"     // 新建候选人
	PermCandidatesCheckin   = "candidates.checkin"    // 候选人签到
	PermCandidatesAssign    = "candidates.assign"     // 拉取候选人进房
	PermRoomsView           = "rooms.view"            // 浏览房间列表/详情
	PermRoomsChat           = "rooms.chat"            // 进房/发消息
	PermRoomsMovePhase      = "rooms.move_phase"      // 推进阶段
	PermRoomsManage         = "rooms.manage"          // 房间成员管理、建空房、删空房
)

// AllPermissions 全部权限名（用于校验角色权限组输入）。
var AllPermissions = []string{
	PermUsersManage,
	PermCandidatesManage,
	PermCandidatesBrowseAll,
	PermCandidatesCreate,
	PermCandidatesCheckin,
	PermCandidatesAssign,
	PermRoomsView,
	PermRoomsChat,
	PermRoomsMovePhase,
	PermRoomsManage,
}

// Role 角色 = 权限组（RBAC）。
type Role struct {
	ID          uint64           `gorm:"primaryKey" json:"id"`
	Name        string           `gorm:"size:64;uniqueIndex" json:"name"`
	Description string           `gorm:"size:255" json:"description"`
	Permissions []RolePermission `gorm:"foreignKey:RoleID" json:"permissions,omitempty"`
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// RolePermission 角色→权限关联（一角色多权限）。
type RolePermission struct {
	ID         uint64 `gorm:"primaryKey" json:"id"`
	RoleID     uint64 `gorm:"uniqueIndex:idx_role_permission" json:"role_id"`
	Permission string `gorm:"size:64;uniqueIndex:idx_role_permission" json:"permission"`
}

// UserRole 用户↔角色 M2M 关联（显式模型，避免 Gorm 隐式 join 表冲突）。
type UserRole struct {
	UserID uint64 `gorm:"uniqueIndex:idx_user_role" json:"user_id"`
	RoleID uint64 `gorm:"uniqueIndex:idx_user_role" json:"role_id"`
}

// TableName 指定关联表名。
func (UserRole) TableName() string { return "user_roles" }

// RolePermission 表名显式指定（默认复数权限名需确认）。
func (RolePermission) TableName() string { return "role_permissions" }
