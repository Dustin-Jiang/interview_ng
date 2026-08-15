package seed

import (
	"context"
	"errors"
	"os"

	"gorm.io/gorm"

	"interview_ng/internal/auth"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/rbac"
)

// adminRoleName / interviewerRoleName 预置角色名。
const (
	adminRoleName       = "admin"
	interviewerRoleName = "interviewer"
)

// Init 启动种子：无角色时创建 admin/interviewer；无用户时创建默认 admin。
// 完成后重载 RBAC 缓存，保证权限即时可用。
func Init(ctx context.Context, db *gorm.DB, cache *rbac.Cache) error {
	var roleCount int64
	if err := db.WithContext(ctx).Model(&dsmodel.Role{}).Count(&roleCount).Error; err != nil {
		return err
	}
	if roleCount == 0 {
		if err := seedRoles(ctx, db); err != nil {
			return err
		}
	}
	var userCount int64
	if err := db.WithContext(ctx).Model(&dsmodel.User{}).Count(&userCount).Error; err != nil {
		return err
	}
	if userCount == 0 {
		if err := seedAdmin(ctx, db); err != nil {
			return err
		}
	}
	return cache.ReloadAll(db)
}

// seedRoles 创建两个预置角色：admin（全 9 权限）、interviewer（6 枚流程权限）。
func seedRoles(ctx context.Context, db *gorm.DB) error {
	admin := &dsmodel.Role{Name: adminRoleName, Description: "管理员：全部权限"}
	interviewer := &dsmodel.Role{Name: interviewerRoleName, Description: "面试官：候选人流程与房间操作"}
	if err := db.WithContext(ctx).Create(admin).Error; err != nil {
		return err
	}
	if err := db.WithContext(ctx).Create(interviewer).Error; err != nil {
		return err
	}
	for _, p := range dsmodel.AllPermissions {
		if err := db.WithContext(ctx).Create(&dsmodel.RolePermission{RoleID: admin.ID, Permission: p}).Error; err != nil {
			return err
		}
	}
	interviewerPerms := []string{
		dsmodel.PermCandidatesCreate,
		dsmodel.PermCandidatesCheckin,
		dsmodel.PermCandidatesAssign,
		dsmodel.PermRoomsView,
		dsmodel.PermRoomsChat,
		dsmodel.PermRoomsMovePhase,
	}
	for _, p := range interviewerPerms {
		if err := db.WithContext(ctx).Create(&dsmodel.RolePermission{RoleID: interviewer.ID, Permission: p}).Error; err != nil {
			return err
		}
	}
	return nil
}

// seedAdmin 创建默认 admin 账号（username=admin，密码取 ADMIN_INIT_PASSWORD，缺省 admin）。
func seedAdmin(ctx context.Context, db *gorm.DB) error {
	pass := os.Getenv("ADMIN_INIT_PASSWORD")
	if pass == "" {
		pass = "admin"
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		return err
	}
	admin := &dsmodel.User{Username: "admin", PasswordHash: hash, Name: "管理员"}
	if err := db.WithContext(ctx).Create(admin).Error; err != nil {
		return err
	}
	var role dsmodel.Role
	if err := db.WithContext(ctx).Where("name = ?", adminRoleName).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("admin role missing, seed roles first")
		}
		return err
	}
	return db.WithContext(ctx).Create(&dsmodel.UserRole{UserID: admin.ID, RoleID: role.ID}).Error
}

// DefaultAdminPassword 返回默认 admin 密码（供日志提示）。
func DefaultAdminPassword() string {
	if p := os.Getenv("ADMIN_INIT_PASSWORD"); p != "" {
		return p
	}
	return "admin"
}
