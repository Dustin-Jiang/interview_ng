package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"interview_ng/internal/auth"
	"interview_ng/internal/broadcast"
	"interview_ng/internal/handler"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/rbac"
	"interview_ng/internal/seed"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

func main() {
	dsn := envOr("DATABASE_DSN", "host=localhost user=postgres password=postgres dbname=interview port=5432 sslmode=disable TimeZone=Asia/Shanghai")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	// 自动建表（演示用；生产建议用迁移工具）。
	if err := db.AutoMigrate(
		&dsmodel.User{}, &dsmodel.Candidate{}, &dsmodel.Room{},
		&dsmodel.RoomMember{}, &dsmodel.Message{},
		&dsmodel.Role{}, &dsmodel.RolePermission{}, &dsmodel.UserRole{},
		&dsmodel.Department{},
	); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// RBAC 内存缓存 + 种子（无角色则建 admin/interviewer，无用户则建默认 admin）。
	cache := rbac.New()
	if err := seed.Init(context.Background(), db, cache); err != nil {
		log.Fatalf("seed: %v", err)
	}

	am := auth.New(envOr("JWT_SECRET", "dev-secret-change-me"), 7*24*time.Hour, db, cache)

	store := state.NewMemStateStore(db)
	b := broadcast.New()
	svc := service.New(store, b)

	httpSrv := handler.NewHTTPServer(svc, store, am)
	wsSrv := handler.NewWSServer(svc, b, store, am)

	r := gin.Default()
	httpSrv.RegisterRoutes(r)
	wsSrv.RegisterRoutes(r)

	addr := envOr("ADDR", ":8080")
	log.Printf("server listening on %s", addr)
	log.Printf("默认 admin 账号: admin / %s（可用 ADMIN_INIT_PASSWORD 覆盖）", seed.DefaultAdminPassword())
	if err := r.Run(addr); err != nil {
		log.Fatalf("run: %v", err)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
