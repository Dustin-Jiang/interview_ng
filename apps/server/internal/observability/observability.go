// Package observability 集中管理后端的指标与日志可观测性：
// OTLP push 直推 GreptimeDB（/v1/otlp/v1/metrics 与 /v1/otlp/v1/logs），
// 未配置 endpoint 时全程走 noop 实现（零开销）。
package observability

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelslog"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	otellogglobal "go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

const otlpBasePath = "/v1/otlp" // GreptimeDB 暴露 OTLP 接口的固定前缀

// Observability 持有进程级指标句柄；全部写操作并发安全。
type Observability struct {
	httpRequests metric.Int64Counter
	httpDuration metric.Float64Histogram
	wsConns      metric.Int64UpDownCounter
	wsMessages   metric.Int64Counter
	wsDropped    metric.Int64Counter
}

// shutdownFn 承担 flush 义务的函数形态（atomic.Value 需要统一具体类型才能换存）。
type shutdownFn func(context.Context) error

// noopShutdown 未配置/失败降级时返回的空 shutdown。
func noopShutdown(context.Context) error { return nil }

// currentPtr 进程级单例，原子读写：Apply 运行时重配在请求 goroutine 中写，
// HTTP/WS 的读方各 goroutine 走 Load——必须同步（Go 内存模型），否则是数据竞争。
// New 成功后替换；此前与降级时均为 Nop。
var currentPtr atomic.Pointer[Observability]

// shutdownHolder 承担 flush 义务的 provider 集合（默认 noop）；与 currentPtr 同期原子替换。
var shutdownHolder atomic.Value // 保存 shutdownFn
func init()                     { baseline() }

// baseline 回到全 noop 基线（进程初始与 Apply 失败降级共用）。
func baseline() {
	currentPtr.Store(newNop())
	shutdownHolder.Store(shutdownFn(noopShutdown))
}

// newNop 全空实现；未配置 endpoint 或初始化失败时使用。
func newNop() *Observability {
	n := &Observability{}
	n.httpRequests, _ = noopMeter().Int64Counter("http_server_requests_total")
	n.httpDuration, _ = noopMeter().Float64Histogram("http_server_request_duration_seconds")
	n.wsConns, _ = noopMeter().Int64UpDownCounter("ws_connections")
	n.wsMessages, _ = noopMeter().Int64Counter("ws_messages_appended")
	n.wsDropped, _ = noopMeter().Int64Counter("ws_messages_dropped")
	return n
}

// applyMu 串行化运行时重配（管理面板保存即生效）；与进程启动的 setup 同一临界区。
// 只锁写方；读方（Current/WS*）走 atomic Load，不取这把锁。
var applyMu sync.Mutex

// Apply 运行时应用一份配置（管理面板的 PUT 路径）：先 flush 旧 provider，
// 再按新配置重建；全部失败降级 Nop（观测关闭、业务不受影响）。
// 返回 (applied, error)：applied=false 时已降级 Nop，err 为 setup 原始错误（供响应体提示）。
func Apply(ctx context.Context, cfg Config) (bool, error) {
	applyMu.Lock()
	defer applyMu.Unlock()
	// flush 旧遥测后回到全 noop 基线，setup 成功则恢复。
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = Shutdown(shutdownCtx)
	currentPtr.Store(newNop())
	shutdownHolder.Store(shutdownFn(noopShutdown))
	if !cfg.Enabled() {
		return true, nil
	}
	if err := setup(ctx, cfg); err != nil {
		slog.Error("observability setup failed, disabled", slog.String("error", err.Error()))
		return false, err
	}
	return true, nil
}

// noopMeter 无 provider 场景下的空 Meter。
func noopMeter() metric.Meter {
	return noop.NewMeterProvider().Meter("interview_ng")
}

// wsRoleCounts 按 role 维护在线连接数的真相值：provider 重建后计数器从 0 起步，
// 若不重放基线，存量连接断开时执行的 -1 会把 gauge 打成负数。
var wsRoleCounts sync.Map // role -> *atomic.Int64

// WSConn 上报 WS 在线连接数变化（gauge，Add +1/-1）。
func WSConn(role string, delta int64) {
	counterOf(role).Add(delta)
	current().wsConn(role, delta)
}

// Current 返回当前生效的 Observability 实例（每请求取一次：面板重配后立即生效）。
func Current() *Observability { return current() }

func current() *Observability { return currentPtr.Load() }

// Shutdown flush 关闭当前生效的 provider（进程退出或 Apply 重配前调用）。
func Shutdown(ctx context.Context) error {
	return shutdownHolder.Load().(shutdownFn)(ctx)
}

// counterOf 取当前 role 的原子在线计数（无则建）。
func counterOf(role string) *atomic.Int64 {
	v, _ := wsRoleCounts.LoadOrStore(role, &atomic.Int64{})
	return v.(*atomic.Int64)
}

// rebaselineWSConns 把重配前已有的在线连接基线重放到新 provider 的计数器上。
func rebaselineWSConns() {
	o := current()
	wsRoleCounts.Range(func(key, v any) bool {
		if n := v.(*atomic.Int64).Load(); n != 0 {
			o.wsConns.Add(context.Background(), n, metric.WithAttributes(roleK(key.(string))))
		}
		return true
	})
}

// WSMessage 上报 WS 已处理消息（op=send_msg|auth 等区分）。
func WSMessage(op string) { current().wsMessage(op) }

// WSDrop 上报写队列缓冲满导致的丢帧（慢消费者）。
func WSDrop(role string) { current().wsDrop(role) }

// setup 真正的初始化：先指标后日志，全部成功才替换进程级单例与 slog 默认 handler。
func setup(ctx context.Context, cfg Config) error {
	res, err := resource.New(ctx, resource.WithAttributes(semconv.ServiceName(cfg.ServiceName)))
	if err != nil {
		return fmt.Errorf("resource: %w", err)
	}
	host, insecure, pathPrefix, ok := cfg.splitEndpoint()
	if !ok {
		return fmt.Errorf("bad OTEL_EXPORTER_OTLP_ENDPOINT %q", cfg.Endpoint)
	}
	// 部署没预建库时先自动补上，失败只告警不停机（GreptimeDB 不从 OTLP 载荷自动建库）。
	ensureDatabase(ctx, cfg)

	headers := cfg.headers()
	timeout := 5 * time.Second

	// --- 指标：OTLP HTTP 直推 GreptimeDB ---
	mOpts := []otlpmetrichttp.Option{
		otlpmetrichttp.WithEndpoint(host),
		otlpmetrichttp.WithURLPath(pathPrefix + otlpBasePath + "/v1/metrics"),
		otlpmetrichttp.WithHeaders(headers),
		otlpmetrichttp.WithTimeout(timeout),
	}
	if insecure {
		mOpts = append(mOpts, otlpmetrichttp.WithInsecure())
	}
	mExp, err := otlpmetrichttp.New(ctx, mOpts...)
	if err != nil {
		return fmt.Errorf("metrics exporter: %w", err)
	}
	mProv := sdkmetric.NewMeterProvider(sdkmetric.WithReader(
		sdkmetric.NewPeriodicReader(mExp, sdkmetric.WithInterval(10*time.Second)),
	))
	otel.SetMeterProvider(mProv)

	// --- 日志：slog 经 bridge 双写（OTLP + stdout）---
	lOpts := []otlploghttp.Option{
		otlploghttp.WithEndpoint(host),
		otlploghttp.WithURLPath(pathPrefix + otlpBasePath + "/v1/logs"),
		otlploghttp.WithHeaders(headers),
		otlploghttp.WithTimeout(timeout),
	}
	if insecure {
		lOpts = append(lOpts, otlploghttp.WithInsecure())
	}
	lExp, err := otlploghttp.New(ctx, lOpts...)
	if err != nil {
		// 回收已建好的指标 provider（其后台导出 goroutine 不能泄漏到全局）。
		shutdownCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		_ = mProv.Shutdown(shutdownCtx)
		return fmt.Errorf("logs exporter: %w", err)
	}
	lProv := log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(lExp, log.WithExportInterval(5*time.Second))),
	)
	otellogglobal.SetLoggerProvider(lProv)

	// bridge handler 转推 OTLP；text handler 保留 stdout 本地日志（双写）。
	slog.SetDefault(slog.New(&slogFanout{handlers: []slog.Handler{
		otelslog.NewHandler(cfg.ServiceName, otelslog.WithLoggerProvider(lProv)),
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}),
	}}))

	currentPtr.Store(newObservability())
	rebaselineWSConns() // 重配场景：存量连接基线重放，避免 gauge 负值
	// 全局 flush：先指标后日志（OTel 推荐顺序，日志可能含指标 flush 出错的告警）。
	shutdownHolder.Store(shutdownFn(func(ctx context.Context) error {
		mErr := mProv.Shutdown(ctx)
		if lErr := lProv.Shutdown(ctx); mErr == nil {
			mErr = lErr
		}
		return mErr
	}))
	return nil
}

// ensureDatabase 确保目标库存在。GreptimeDB 的「自动生成表结构」只覆盖**表**（写入时建表、
// 补列），**不建库**：往不存在的库推 OTLP 一律 400 `Failed to find schema`，且引擎没有
// 「自动建库」配置项（`auto_create_schema` 只针对 PostgreSQL 元数据库的 schema）。
// 实测 0.11.0 与 1.2.1 一致（OTLP metrics / logs、InfluxDB 行协议、`/v1/sql` 三条写入路径都不建库），
// 故这里先补一条 CREATE DATABASE IF NOT EXISTS（两版本都幂等可用）——删掉它会让
// 「面板里换个新库名」直接失效，只剩每 5~10s 一次的推送失败日志。
// 带与 OTLP 推送同口径的 Basic 鉴权头（开了鉴权时本请求也要过认证）；连接失败/无权限
// 都不致命（告警后继续——后续 OTLP 推送会持续报错但业务不受影响）。
func ensureDatabase(ctx context.Context, cfg Config) {
	body := "sql=" + url.QueryEscape("CREATE DATABASE IF NOT EXISTS "+cfg.Database)
	req, err := http.NewRequest(http.MethodPost, strings.TrimRight(cfg.Endpoint, "/")+"/v1/sql", strings.NewReader(body))
	if err != nil {
		slog.Error("ensureDatabase build request failed", slog.String("error", err.Error()))
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if cfg.Username != "" || cfg.Password != "" {
		req.Header.Set("Authorization",
			"Basic "+base64.StdEncoding.EncodeToString([]byte(cfg.Username+":"+cfg.Password)))
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		slog.Error("ensureDatabase request failed", slog.String("error", err.Error()))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= http.StatusBadRequest {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		slog.Warn("ensureDatabase non-2xx", slog.Int("status", resp.StatusCode), slog.String("body", string(b)))
	}
}

// newObservability 注册进程级全部指标句柄；noop 场景必然成功。
// 已配置场景的单个注册失败只能忽略：丢了该指标也好过观测故障打挂主进程。
func newObservability() *Observability {
	m := otel.GetMeterProvider().Meter("interview_ng")
	o := &Observability{}
	o.httpRequests, _ = m.Int64Counter("http_server_requests_total")
	o.httpDuration, _ = m.Float64Histogram("http_server_request_duration_seconds")
	o.wsConns, _ = m.Int64UpDownCounter("ws_connections")
	o.wsMessages, _ = m.Int64Counter("ws_messages_appended")
	o.wsDropped, _ = m.Int64Counter("ws_messages_dropped")
	return o
}

func (o *Observability) wsConn(role string, delta int64) {
	o.wsConns.Add(context.Background(), delta, metric.WithAttributes(roleK(role)))
}

func (o *Observability) wsMessage(op string) {
	o.wsMessages.Add(context.Background(), 1, metric.WithAttributes(opK(op)))
}

func (o *Observability) wsDrop(role string) {
	o.wsDropped.Add(context.Background(), 1, metric.WithAttributes(roleK(role)))
}

// slogFanout 把一条记录分发给多个 handler（logs 双写 stdout + GreptimeDB）。
type slogFanout struct{ handlers []slog.Handler }

func (f *slogFanout) Enabled(ctx context.Context, l slog.Level) bool {
	for _, h := range f.handlers {
		if h.Enabled(ctx, l) {
			return true
		}
	}
	return false
}

func (f *slogFanout) Handle(ctx context.Context, r slog.Record) error {
	var firstErr error
	for _, h := range f.handlers {
		if err := h.Handle(ctx, r.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (f *slogFanout) WithAttrs(attrs []slog.Attr) slog.Handler {
	next := &slogFanout{handlers: make([]slog.Handler, 0, len(f.handlers))}
	for _, h := range f.handlers {
		next.handlers = append(next.handlers, h.WithAttrs(attrs))
	}
	return next
}

func (f *slogFanout) WithGroup(name string) slog.Handler {
	next := &slogFanout{handlers: make([]slog.Handler, 0, len(f.handlers))}
	for _, h := range f.handlers {
		next.handlers = append(next.handlers, h.WithGroup(name))
	}
	return next
}
