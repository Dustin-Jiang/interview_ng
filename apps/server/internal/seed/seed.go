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

// adminRoleName / interviewerRoleName 预置角色名；defaultDeptName 预置部门名。
const (
	adminRoleName       = "admin"
	interviewerRoleName = "interviewer"
	defaultDeptName     = "默认部门"
)

// Init 启动种子：无角色时创建 admin/interviewer；无部门时创建默认部门；无用户时创建默认 admin。
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
	var deptCount int64
	if err := db.WithContext(ctx).Model(&dsmodel.Department{}).Count(&deptCount).Error; err != nil {
		return err
	}
	if deptCount == 0 {
		if _, err := seedDefaultDepartment(ctx, db); err != nil {
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
	// 幂等收尾：给 admin 角色补齐目录新增的权限（已有库升级路径），
	// 并保证每位面试官都有部门（无部门用户归入默认部门）。
	if err := reconcileAdminRole(ctx, db); err != nil {
		return err
	}
	if err := reconcileUsersDepartment(ctx, db); err != nil {
		return err
	}
	return cache.ReloadAll(db)
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

// reconcileUsersDepartment 把无部门面试官归入默认部门（保证每位面试官都有部门）。
// admin 是全局系统角色，不隶属任何部门：升级路径中把已有 admin 的部门清空。
func reconcileUsersDepartment(ctx context.Context, db *gorm.DB) error {
	// 找出 admin 角色的用户 id。
	var adminIDs []uint64
	if err := db.WithContext(ctx).Model(&dsmodel.UserRole{}).
		Joins("JOIN roles ON roles.id = user_roles.role_id").
		Where("roles.name = ?", adminRoleName).
		Pluck("user_roles.user_id", &adminIDs).Error; err != nil {
		return err
	}
	if len(adminIDs) > 0 {
		// admin 不隶属任何部门：清空其部门归属。
		if err := db.WithContext(ctx).Model(&dsmodel.User{}).
			Where("id IN ?", adminIDs).Update("department_id", nil).Error; err != nil {
			return err
		}
	}
	// 余下无部门用户（面试官，不含 admin）归入默认部门。
	q := db.WithContext(ctx).Select("id").Where("department_id IS NULL")
	if len(adminIDs) > 0 {
		q.Where("id NOT IN ?", adminIDs)
	}
	var noDept []dsmodel.User
	if err := q.Find(&noDept).Error; err != nil {
		return err
	}
	if len(noDept) == 0 {
		return nil
	}
	dept, err := seedDefaultDepartment(ctx, db)
	if err != nil {
		return err
	}
	for _, u := range noDept {
		if err := db.WithContext(ctx).Model(&dsmodel.User{}).Where("id = ?", u.ID).
			Update("department_id", dept.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

// seedRoles 创建两个预置角色：admin（全权限）、interviewer（6 枚流程权限）。
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
// admin 是全局系统角色，不隶属任何部门（不归默认部门）。
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

// seedDefaultDepartment 无部门时创建默认部门；已存在则返回现有记录（幂等）。
func seedDefaultDepartment(ctx context.Context, db *gorm.DB) (*dsmodel.Department, error) {
	var dept dsmodel.Department
	if err := db.WithContext(ctx).Where("name = ?", defaultDeptName).First(&dept).Error; err == nil {
		return &dept, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	dept = dsmodel.Department{Name: defaultDeptName, Description: "系统默认部门"}
	if err := db.WithContext(ctx).Create(&dept).Error; err != nil {
		return nil, err
	}
	return &dept, nil
}

// DefaultAdminPassword 返回默认 admin 密码（供日志提示）。
func DefaultAdminPassword() string {
	if p := os.Getenv("ADMIN_INIT_PASSWORD"); p != "" {
		return p
	}
	return "admin"
}
