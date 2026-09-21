package seed

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/rbac"
)

func memDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&dsmodel.User{}, &dsmodel.Candidate{}, &dsmodel.Room{},
		&dsmodel.RoomMember{}, &dsmodel.Message{},
		&dsmodel.Role{}, &dsmodel.RolePermission{}, &dsmodel.UserRole{},
		&dsmodel.Department{}, &dsmodel.SystemStatus{}, &dsmodel.CandidateAdmission{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestSeedUpgradeReconcile 已有库（admin/角色已存在且缺新权限、用户无部门）重启时的幂等补齐：
// admin 角色补上新增权限；admin 不隶属部门（部门清空）；非 admin 无部门用户归入默认部门。
func TestSeedUpgradeReconcile(t *testing.T) {
	ctx := context.Background()
	db := memDB(t)

	// 模拟旧库：admin 角色（缺新权限 browse_all）、一个已被分配部门的 admin 用户、
	// 一个无部门的普通面试官；无部门表记录（由 reconcile 幂等创建）。
	adminRole := &dsmodel.Role{Name: adminRoleName, Description: ""}
	if err := db.Create(adminRole).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	// 旧库的 admin 角色只持有旧权限集的子集（模拟目录新增前）
	if err := db.Create(&dsmodel.RolePermission{RoleID: adminRole.ID, Permission: dsmodel.PermUsersManage}).Error; err != nil {
		t.Fatalf("seed old perm: %v", err)
	}
	// 旧库曾把 admin 归入默认部门
	oldDept := &dsmodel.Department{Name: defaultDeptName, Description: ""}
	if err := db.Create(oldDept).Error; err != nil {
		t.Fatalf("create old dept: %v", err)
	}
	admin := &dsmodel.User{Username: "admin", Name: "管理员", PasswordHash: "x", DepartmentID: &oldDept.ID}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := db.Create(&dsmodel.UserRole{UserID: admin.ID, RoleID: adminRole.ID}).Error; err != nil {
		t.Fatalf("create userrole: %v", err)
	}
	itv := &dsmodel.User{Username: "itv", Name: "面试官", PasswordHash: "x"}
	if err := db.Create(itv).Error; err != nil {
		t.Fatalf("create interviewer: %v", err)
	}

	cache := rbac.New()
	if err := Init(ctx, db, cache); err != nil {
		t.Fatalf("init: %v", err)
	}

	// admin 角色应补齐全部权限（含新增的 browse_all）
	var perms []string
	if err := db.Model(&dsmodel.RolePermission{}).Where("role_id = ?", adminRole.ID).
		Pluck("permission", &perms).Error; err != nil {
		t.Fatalf("pluck perms: %v", err)
	}
	have := make(map[string]bool, len(perms))
	for _, p := range perms {
		have[p] = true
	}
	for _, p := range dsmodel.AllPermissions {
		if !have[p] {
			t.Fatalf("admin role missing permission %s after reconcile", p)
		}
	}

	// admin 不隶属任何部门：原有部门应被清空
	var a dsmodel.User
	if err := db.First(&a, admin.ID).Error; err != nil {
		t.Fatalf("get admin: %v", err)
	}
	if a.DepartmentID != nil {
		t.Fatalf("admin should not belong to any department, got %d", *a.DepartmentID)
	}

	// 非 admin 无部门用户应补上默认部门
	var u dsmodel.User
	if err := db.First(&u, itv.ID).Error; err != nil {
		t.Fatalf("get interviewer: %v", err)
	}
	if u.DepartmentID == nil {
		t.Fatalf("interviewer should be assigned to default department after reconcile")
	}
	dept, err := seedDefaultDepartment(ctx, db)
	if err != nil {
		t.Fatalf("default department: %v", err)
	}
	if *u.DepartmentID != dept.ID {
		t.Fatalf("interviewer dept=%d, want default=%d", *u.DepartmentID, dept.ID)
	}

	// 幂等：再次 Init 不报错、不重复插部门、admin 仍无部门、面试官仍归默认部门
	var deptBefore int64
	db.Model(&dsmodel.Department{}).Count(&deptBefore)
	var deptAfter int64
	if err := Init(ctx, db, cache); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	db.Model(&dsmodel.Department{}).Count(&deptAfter)
	if deptAfter != deptBefore {
		t.Fatalf("re-init should not create extra departments: %d -> %d", deptBefore, deptAfter)
	}
	if err := db.First(&a, admin.ID).Error; err == nil && a.DepartmentID != nil {
		t.Fatalf("admin should stay dept-less after re-init, got %d", *a.DepartmentID)
	}
	if err := db.First(&u, itv.ID).Error; err == nil && (u.DepartmentID == nil || *u.DepartmentID != dept.ID) {
		t.Fatalf("interviewer dept should persist after re-init: %+v", u)
	}
}

// TestSeedAdminHasNoDepartment 全新空库种子：admin 创建后不隶属任何部门。
func TestSeedAdminHasNoDepartment(t *testing.T) {
	ctx := context.Background()
	db := memDB(t)
	cache := rbac.New()
	if err := Init(ctx, db, cache); err != nil {
		t.Fatalf("init: %v", err)
	}
	var admin dsmodel.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatalf("admin not seeded: %v", err)
	}
	if admin.DepartmentID != nil {
		t.Fatalf("new admin should have no department, got %d", *admin.DepartmentID)
	}
}
