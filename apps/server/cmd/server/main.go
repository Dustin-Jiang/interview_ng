package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"interview_ng/internal/auth"
	"interview_ng/internal/broadcast"
	"interview_ng/internal/handler"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/oidcauth"
	"interview_ng/internal/rbac"
	"interview_ng/internal/seed"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

func main() {
	dsn := envOr("DATABASE_DSN", "host=localhost user=postgres password=postgres dbname=interview port=5432 sslmode=disable TimeZone=Asia/Shanghai")
	db, err := openDBWithRetry(dsn, connectTimeout())
	if err != nil {
		log.Fatalf("open db: %v", err)
	}

	// 自动建表（演示用；生产建议用迁移工具）。
	if err := db.AutoMigrate(
		&dsmodel.User{}, &dsmodel.Candidate{}, &dsmodel.Room{},
		&dsmodel.RoomMember{}, &dsmodel.Message{},
		&dsmodel.Role{}, &dsmodel.RolePermission{}, &dsmodel.UserRole{},
		&dsmodel.Department{}, &dsmodel.SystemStatus{}, &dsmodel.CandidateAdmission{}, &dsmodel.Bid{},
		&dsmodel.OidcConfig{}, &dsmodel.OidcRoleRule{}, &dsmodel.OidcDeptRule{},
	); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// RBAC 内存缓存 + 种子（无角色则建 admin/interviewer，无用户则建默认 admin）。
	cache := rbac.New()
	if err := seed.Init(context.Background(), db, cache); err != nil {
		log.Fatalf("seed: %v", err)
	}

	store := state.NewMemStateStore(db)
	am := auth.New(envOr("JWT_SECRET", "dev-secret-change-me"), 7*24*time.Hour, db, cache, store)

	b := broadcast.New()
	svc := service.New(store, b)
	oidcSvc := oidcauth.New()

	httpSrv := handler.NewHTTPServer(svc, store, am, oidcSvc)
	wsSrv := handler.NewWSServer(svc, b, store, am)

	r := gin.Default()
	httpSrv.RegisterRoutes(r)
	wsSrv.RegisterRoutes(r)

	// 前端产物：给了 WEB_ROOT 且目录里有 index.html 时由本进程直接提供（单进程部署）。
	// 未设置则只提供 /api 与 /ws，前端仍走 Vite dev server（:3000）。
	if root := os.Getenv("WEB_ROOT"); handler.RegisterStatic(r, root) {
		log.Printf("serving web assets from %s", root)
	}

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

// openDBWithRetry 连不上数据库时按秒重试，直到 timeout（timeout <= 0 则只试一次）。
//
// 为什么需要：后端常常先于 Postgres 就绪——compose 的 depends_on 只保证容器创建顺序，
// podman-compose 也不等 healthcheck 变健康，而首次启动还要 initdb；而 gorm.Open 是**立即**
// 建连的，连不上就直接返回错误（原先是 Fatalf 退出）。把等待放在进程里，部署方式
// （compose / 裸机 / k8s）都不必再操心启动顺序，也不再依赖外层脚本。
func openDBWithRetry(dsn string, timeout time.Duration) (*gorm.DB, error) {
	deadline := time.Now().Add(timeout)
	for attempt := 1; ; attempt++ {
		db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logger.Warn),
		})
		if err == nil {
			if attempt > 1 {
				log.Printf("数据库已就绪（第 %d 次尝试）", attempt)
			}
			return db, nil
		}
		if timeout <= 0 || time.Now().After(deadline) {
			return nil, err
		}
		log.Printf("连接数据库失败（第 %d 次，剩余 %.0fs 内继续重试）: %v", attempt, time.Until(deadline).Seconds(), err)
		time.Sleep(time.Second)
	}
}

// connectTimeout：DB_CONNECT_TIMEOUT 支持 "60s" 这类时长或纯秒数；缺省 60s，<=0 表示不重试。
func connectTimeout() time.Duration {
	v := os.Getenv("DB_CONNECT_TIMEOUT")
	if v == "" {
		return 60 * time.Second
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return 60 * time.Second
}
