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
		&dsmodel.OidcConfig{}, &dsmodel.OidcRoleRule{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestSeedUpgradeReconcile 已有库（admin 角色缺新权限、admin 曾归属部门）重启时的幂等补齐：
// admin 角色补上新增权限；admin 的部门归属被清空；**不预置部门、也不给无部门用户补部门**。
func TestSeedUpgradeReconcile(t *testing.T) {
	ctx := context.Background()
	db := memDB(t)

	// 模拟旧库：admin 角色（只持旧权限子集）、一个已被归入某部门的 admin 用户、一个无部门的面试官。
	adminRole := &dsmodel.Role{Name: adminRoleName, Description: ""}
	if err := db.Create(adminRole).Error; err != nil {
		t.Fatalf("create role: %v", err)
	}
	if err := db.Create(&dsmodel.RolePermission{RoleID: adminRole.ID, Permission: dsmodel.PermUsersManage}).Error; err != nil {
		t.Fatalf("seed old perm: %v", err)
	}
	oldDept := &dsmodel.Department{Name: "旧部门", Description: ""}
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

	// admin 角色应补齐全部权限
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

	// admin 不隶属任何部门：原有部门归属应被清空
	var a dsmodel.User
	if err := db.First(&a, admin.ID).Error; err != nil {
		t.Fatalf("get admin: %v", err)
	}
	if a.DepartmentID != nil {
		t.Fatalf("admin should not belong to any department, got %d", *a.DepartmentID)
	}

	// 面试官保持无部门：种子不预置部门，也不补部门归属
	var u dsmodel.User
	if err := db.First(&u, itv.ID).Error; err != nil {
		t.Fatalf("get interviewer: %v", err)
	}
	if u.DepartmentID != nil {
		t.Fatalf("interviewer should stay dept-less, got %d", *u.DepartmentID)
	}

	// 幂等：再次 Init 不报错、不新建部门、admin 与面试官仍无部门
	var deptBefore int64
	db.Model(&dsmodel.Department{}).Count(&deptBefore)
	if err := Init(ctx, db, cache); err != nil {
		t.Fatalf("re-init: %v", err)
	}
	var deptAfter int64
	db.Model(&dsmodel.Department{}).Count(&deptAfter)
	if deptAfter != deptBefore {
		t.Fatalf("re-init should not create departments: %d -> %d", deptBefore, deptAfter)
	}
	if err := db.First(&a, admin.ID).Error; err == nil && a.DepartmentID != nil {
		t.Fatalf("admin should stay dept-less after re-init, got %d", *a.DepartmentID)
	}
	if err := db.First(&u, itv.ID).Error; err == nil && u.DepartmentID != nil {
		t.Fatalf("interviewer should stay dept-less after re-init, got %d", *u.DepartmentID)
	}
}

// TestSeedFreshDBCreatesNoDepartment 全新空库种子：不预置任何部门；admin 不隶属部门；只建两个预置角色。
func TestSeedFreshDBCreatesNoDepartment(t *testing.T) {
	ctx := context.Background()
	db := memDB(t)
	cache := rbac.New()
	if err := Init(ctx, db, cache); err != nil {
		t.Fatalf("init: %v", err)
	}
	var deptCount int64
	db.Model(&dsmodel.Department{}).Count(&deptCount)
	if deptCount != 0 {
		t.Fatalf("空库种子不应创建部门，实际 %d 个", deptCount)
	}
	var admin dsmodel.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err != nil {
		t.Fatalf("admin not seeded: %v", err)
	}
	if admin.DepartmentID != nil {
		t.Fatalf("new admin should have no department, got %d", *admin.DepartmentID)
	}
	var roleCount int64
	db.Model(&dsmodel.Role{}).Count(&roleCount)
	if roleCount != 2 {
		t.Fatalf("预置角色应为 2 个，实际 %d 个", roleCount)
	}
}
