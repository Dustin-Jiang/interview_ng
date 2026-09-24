package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"interview_ng/internal/auth"
	"interview_ng/internal/broadcast"
	"interview_ng/internal/handler"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/observability"
	"interview_ng/internal/oidcauth"
	"interview_ng/internal/rbac"
	"interview_ng/internal/seed"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = observability.Shutdown(shutdownCtx)
	}()
	dsn := envOr("DATABASE_DSN", "host=localhost user=postgres password=postgres dbname=interview port=5432 sslmode=disable TimeZone=Asia/Shanghai")

	db, err := openDBWithRetry(dsn, connectTimeout())
	if err != nil {
		slog.Error("open db", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// 自动建表（演示用；生产建议用迁移工具）。
	if err := db.AutoMigrate(
		&dsmodel.User{}, &dsmodel.Candidate{}, &dsmodel.Room{},
		&dsmodel.RoomMember{}, &dsmodel.Message{}, &dsmodel.MessageReaction{},
		&dsmodel.Role{}, &dsmodel.RolePermission{}, &dsmodel.UserRole{},
		&dsmodel.Department{}, &dsmodel.SystemStatus{}, &dsmodel.CandidateAdmission{}, &dsmodel.Bid{},
		&dsmodel.OidcConfig{}, &dsmodel.OidcRoleRule{}, &dsmodel.OidcDeptRule{},
		&dsmodel.ObservabilityConfig{},
	); err != nil {
		slog.Error("migrate", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// RBAC 内存缓存 + 种子（无角色则建 admin/interviewer，无用户则建默认 admin）。
	cache := rbac.New()
	if err := seed.Init(context.Background(), db, cache); err != nil {
		slog.Error("seed", slog.String("error", err.Error()))
		os.Exit(1)
	}

	store := state.NewMemStateStore(db)

	// 可观测性配置：数据库单行是运行时权威（管理面板保存即生效，无需重启）。
	// 环境变量只在「库里从没配过 endpoint」时作首次种子（compose 的 env 注入仍开箱即用）。
	if err := seedObservability(store); err != nil {
		slog.Error("observability config seeding", slog.String("error", err.Error()))
	}
	apCfg, err := store.GetObservabilityConfig(context.Background())
	if err != nil {
		slog.Error("observability config load", slog.String("error", err.Error()))
	}
	applied := false
	if apCfg != nil && apCfg.Enabled {
		applied, _ = observability.Apply(context.Background(), observability.Config{
			Endpoint:    apCfg.Endpoint,
			Database:    apCfg.Database,
			Username:    apCfg.Username,
			Password:    apCfg.Password,
			ServiceName: apCfg.ServiceName,
		})
	}
	// 未启用或失败 → 空配置走关闭分支（flush 旧遥测后保持 noop）。
	if !applied {
		_, _ = observability.Apply(context.Background(), observability.Config{})
	}
	if apCfg != nil {
		slog.Info("observability", slog.Bool("enabled", apCfg.Enabled), slog.String("endpoint", apCfg.Endpoint))
	}

	secret := os.Getenv("JWT_SECRET")
	if secret == "" || secret == "dev-secret-change-me" {
		// 占位密钥下任何知道仓库的人都能离线伪造合法 JWT：内网开发可接受，生产必须显式配置。
		slog.Warn("警告: JWT_SECRET 未设置或等于开发占位值，任何人可伪造 token；仅限内网/开发使用")
		secret = "dev-secret-change-me"
	}
	am := auth.New(secret, 7*24*time.Hour, db, cache, store)

	b := broadcast.New()
	svc := service.New(store, b)
	oidcSvc := oidcauth.New()

	httpSrv := handler.NewHTTPServer(svc, store, am, oidcSvc)
	wsSrv := handler.NewWSServer(svc, b, store, am)
	// RED 中间件 + Recovery（替代 gin.Default 的 Logger/Recovery；
	// access log 由 observability 中间件的指标 + slog bridge 双写承担）。
	r := gin.New()
	observability.Install(r)
	httpSrv.RegisterRoutes(r)
	wsSrv.RegisterRoutes(r)

	// 前端产物：给了 WEB_ROOT 且目录里有 index.html 时由本进程直接提供（单进程部署）。
	// 未设置则只提供 /api 与 /ws，前端仍走 Vite dev server（:3000）。
	if root := os.Getenv("WEB_ROOT"); handler.RegisterStatic(r, root) {
		slog.Info("serving web assets", slog.String("root", root))
	}

	slog.Info("默认 admin 账号: admin（口令由 ADMIN_INIT_PASSWORD 指定）")
	addr := envOr("ADDR", ":8080")
	slog.Info("server listening", slog.String("addr", addr))
	srv := &http.Server{Addr: addr, Handler: r, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	// 优雅退出：SIGTERM/SIGINT → 5s 排空在途请求；src 监听错误走 errCh 立即退出。
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()
	select {
	case err := <-errCh:
		slog.Error("run", slog.String("error", err.Error()))
		os.Exit(1)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			slog.Error("graceful shutdown", slog.String("error", err.Error()))
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// seedObservability 环境变量首次种子：仅当库里配置从未配过 endpoint 且开关关闭时，
// 把 compose 注入的 OTEL_EXPORTER_OTLP_* 环境变量写入数据库单行（此后管理面板是权威）。
func seedObservability(store state.StateStore) error {
	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		return nil
	}
	cur, err := store.GetObservabilityConfig(context.Background())
	if err != nil {
		return err
	}
	if cur.Enabled || cur.Endpoint != "" {
		return nil // 已由面板配置或此前 seeding 过：环境变量不再覆盖
	}
	cfg := &dsmodel.ObservabilityConfig{
		Enabled:     true,
		Endpoint:    os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"),
		Database:    envOr("OTEL_EXPORTER_OTLP_DATABASE", "interview_ng"),
		Username:    envOr("OTEL_EXPORTER_OTLP_USERNAME", "greptime"),
		Password:    os.Getenv("OTEL_EXPORTER_OTLP_PASSWORD"),
		ServiceName: envOr("OTEL_SERVICE_NAME", "interview_ng"),
	}
	return store.SetObservabilityConfig(context.Background(), cfg, &cfg.Password)
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
				slog.Info(fmt.Sprintf("数据库已就绪（第 %d 次尝试）", attempt))
			}
			return db, nil
		}
		if timeout <= 0 || time.Now().After(deadline) {
			return nil, err
		}
		slog.Warn(fmt.Sprintf("连接数据库失败（第 %d 次，剩余 %.0fs 内继续重试）: %v", attempt, time.Until(deadline).Seconds(), err))
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
