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
// **不预置任何部门**——部门与用户归属由管理员显式创建/分配。
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
	// 幂等收尾：给 admin 角色补齐目录新增的权限、给 interviewer 角色补齐预置权限
	// （已有库升级路径），并清空 admin 的部门归属（admin 不隶属任何部门）。
	if err := reconcileAdminRole(ctx, db); err != nil {
		return err
	}
	if err := reconcileInterviewerRole(ctx, db); err != nil {
		return err
	}
	if err := reconcileAdminDepartment(ctx, db); err != nil {
		return err
	}
	return cache.ReloadAll(db)
}

// reconcileInterviewerRole 给 interviewer 角色补齐预置权限中缺失的项
// （预置权限目录扩展后，已有库无需重建即可生效）。
func reconcileInterviewerRole(ctx context.Context, db *gorm.DB) error {
	var role dsmodel.Role
	if err := db.WithContext(ctx).Where("name = ?", interviewerRoleName).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var have []string
	if err := db.WithContext(ctx).Model(&dsmodel.RolePermission{}).
		Where("role_id = ?", role.ID).Pluck("permission", &have).Error; err != nil {
		return err
	}
	haveSet := make(map[string]bool, len(have))
	for _, p := range have {
		haveSet[p] = true
	}
	for _, p := range interviewerPerms() {
		if haveSet[p] {
			continue
		}
		if err := db.WithContext(ctx).Create(&dsmodel.RolePermission{RoleID: role.ID, Permission: p}).Error; err != nil {
			return err
		}
	}
	return nil
}

// reconcileAdminRole 给 admin 角色补齐 AllPermissions 中缺失的权限
// （新增权限目录后，无需要空库重播种即可在已有库即时生效）。
func reconcileAdminRole(ctx context.Context, db *gorm.DB) error {
	var role dsmodel.Role
	if err := db.WithContext(ctx).Where("name = ?", adminRoleName).First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	}
	var have []string
	if err := db.WithContext(ctx).Model(&dsmodel.RolePermission{}).
		Where("role_id = ?", role.ID).Pluck("permission", &have).Error; err != nil {
		return err
	}
	haveSet := make(map[string]bool, len(have))
	for _, p := range have {
		haveSet[p] = true
	}
	for _, p := range dsmodel.AllPermissions {
		if haveSet[p] {
			continue
		}
		if err := db.WithContext(ctx).Create(&dsmodel.RolePermission{RoleID: role.ID, Permission: p}).Error; err != nil {
			return err
		}
	}
	return nil
}

// reconcileAdminDepartment 清空 admin 用户的部门归属：admin 是全局系统角色，不隶属任何部门
// （旧库升级路径曾把 admin 归入默认部门）。**不预置、也不补任何部门**。
func reconcileAdminDepartment(ctx context.Context, db *gorm.DB) error {
	var adminIDs []uint64
	if err := db.WithContext(ctx).Model(&dsmodel.UserRole{}).
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.name = ?", adminRoleName).
		Pluck("user_roles.user_id", &adminIDs).Error; err != nil {
		return err
	}
	if len(adminIDs) == 0 {
		return nil
	}
	// admin 不隶属任何部门：清空其部门归属。
	return db.WithContext(ctx).Model(&dsmodel.User{}).
		Where("id IN ?", adminIDs).Update("department_id", nil).Error
}

// seedRoles 创建两个预置角色：admin（全权限）、interviewer（8 枚流程权限）。
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
	for _, p := range interviewerPerms() {
		if err := db.WithContext(ctx).Create(&dsmodel.RolePermission{RoleID: interviewer.ID, Permission: p}).Error; err != nil {
			return err
		}
	}
	return nil
}

// interviewerPerms 面试官预置权限（候选人流程 + 房间操作 + 志愿调剂 + 本部门录取记录）。
func interviewerPerms() []string {
	return []string{
		dsmodel.PermCandidatesCreate,
		dsmodel.PermCandidatesCheckin,
		dsmodel.PermCandidatesAssign,
		dsmodel.PermCandidatesPreferences,
		dsmodel.PermRoomsView,
		dsmodel.PermRoomsChat,
		dsmodel.PermRoomsMovePhase,
		dsmodel.PermAdmissionRecord,
	}
}

// seedAdmin 创建默认 admin 账号（username=admin，密码取 ADMIN_INIT_PASSWORD，缺省 admin）。
// admin 是全局系统角色，不隶属任何部门。
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
