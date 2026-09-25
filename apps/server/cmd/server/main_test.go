package main

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	dsmodel "interview_ng/internal/model"
)

func newMigrateTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}

// TestDBResetFlag 只有显式的真值才开启删表重建：其余（含未设置、拼错的取值）一律保持默认的拒绝启动。
func TestDBResetFlag(t *testing.T) {
	for _, raw := range []string{"1", "true", "TRUE", "yes", "on", " on "} {
		t.Setenv("DB_RESET", raw)
		if !dbReset() {
			t.Fatalf("DB_RESET=%q 应开启重置", raw)
		}
	}
	for _, raw := range []string{"", "0", "false", "no", "off", "2", "reset"} {
		t.Setenv("DB_RESET", raw)
		if dbReset() {
			t.Fatalf("DB_RESET=%q 不应开启重置（自动清库会删掉真实数据）", raw)
		}
	}
}

// TestIncompatibleSchemaHint 只认「candidates 在但缺 student_no」这一种 AutoMigrate 改不动的旧库形状：
// 空库/已迁移的库都给空提示（不打扰正常启动）。
func TestIncompatibleSchemaHint(t *testing.T) {
	t.Run("空库", func(t *testing.T) {
		if hint := incompatibleSchemaHint(newMigrateTestDB(t)); hint != "" {
			t.Fatalf("空库不应给出提示，得到 %q", hint)
		}
	})

	t.Run("已迁移", func(t *testing.T) {
		db := newMigrateTestDB(t)
		if err := db.AutoMigrate(appModels()...); err != nil {
			t.Fatalf("migrate: %v", err)
		}
		if hint := incompatibleSchemaHint(db); hint != "" {
			t.Fatalf("已迁移的库不应给出提示，得到 %q", hint)
		}
	})

	t.Run("旧库缺学号列", func(t *testing.T) {
		db := newMigrateTestDB(t)
		// 伪造 student_no 之前的 candidates 表（有数据，NOT NULL 列补不上去）。
		if err := db.Exec(`create table candidates (id integer primary key autoincrement, name text, status text)`).Error; err != nil {
			t.Fatalf("create legacy candidates: %v", err)
		}
		if err := db.Exec(`insert into candidates (name, status) values ('旧数据', 'COMPLETED')`).Error; err != nil {
			t.Fatalf("insert legacy candidate: %v", err)
		}
		hint := incompatibleSchemaHint(db)
		if !strings.Contains(hint, "student_no") || !strings.Contains(hint, "DB_RESET=1") {
			t.Fatalf("提示应说明缺列与重建办法，得到 %q", hint)
		}
	})
}

// TestDropAppTablesThenMigrate 是 DB_RESET=1 的落库口径：删表必须能一次删掉带外键的全套表，
// 之后的 AutoMigrate 建回完整 schema（含 NOT NULL 的 student_no）并可正常写入。
func TestDropAppTablesThenMigrate(t *testing.T) {
	db := newMigrateTestDB(t)
	if err := db.AutoMigrate(appModels()...); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&dsmodel.User{Username: "admin", PasswordHash: "x"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&dsmodel.Candidate{StudentNo: "001", Name: "甲"}).Error; err != nil {
		t.Fatalf("seed candidate: %v", err)
	}

	if err := db.Migrator().DropTable(appModels()...); err != nil {
		t.Fatalf("drop tables: %v", err)
	}
	if err := db.AutoMigrate(appModels()...); err != nil {
		t.Fatalf("re-migrate: %v", err)
	}

	var users, candidates int64
	db.Model(&dsmodel.User{}).Count(&users)
	db.Model(&dsmodel.Candidate{}).Count(&candidates)
	if users != 0 || candidates != 0 {
		t.Fatalf("重建后应为空库: users=%d candidates=%d", users, candidates)
	}
	if err := db.Create(&dsmodel.Candidate{StudentNo: "002", Name: "乙"}).Error; err != nil {
		t.Fatalf("重建后写入: %v", err)
	}
	// student_no 的 NOT NULL 约束必须还在（空学号写不进去）。
	if err := db.Exec(`insert into candidates (name, status) values ('无学号', 'NOT_CHECKED_IN')`).Error; err == nil {
		t.Fatal("重建后 student_no 仍应是 NOT NULL")
	}
}
