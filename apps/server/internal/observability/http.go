package observability

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// 属性 key：语义对齐 OTel 约定，GreptimeDB 落库同名列。
var (
	keyMethod = attribute.Key("http.request.method")
	keyRoute  = attribute.Key("http.route")
	keyCode   = attribute.Key("http.response.status_code")
	keyOp     = attribute.Key("op")
	keyRole   = attribute.Key("role")
)

func mString(k attribute.Key, v string) attribute.KeyValue { return k.String(v) }

func hMethod(v string) attribute.KeyValue { return mString(keyMethod, v) }
func hRoute(v string) attribute.KeyValue  { return mString(keyRoute, v) }
func hCode(v int) attribute.KeyValue      { return mString(keyCode, itoa(v)) }

func opK(v string) attribute.KeyValue    { return keyOp.String(v) }
func opRole(v string) attribute.KeyValue { return keyRole.String("ws." + v) }
func roleK(v string) attribute.KeyValue  { return keyRole.String(v) }

// Install 把 RED 中间件与 gin.Recovery 挂到引擎
// （替代 gin.Default 的 Logger/Recovery：access log 由 RED 指标 + slog bridge 承担）。
// 必须在 New 执行（成功或降级为 Nop）之后调用——Nop 状态下也安全可挂。
func Install(r *gin.Engine) { r.Use(Middleware(), gin.Recovery()) }

// Middleware HTTP RED 中间件：请求计数 + 时延直方图。
// attrs：method / route（c.FullPath()，未匹配为 "unmatched"）/ st_code（仅计数）。
// 每请求重新取 Current()：管理面板保存重配后新 provider 立即生效，无需重装中间件。
func Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		o := Current()
		// WS 路由的 handler 会阻塞在 <quit> 到整段连接结束：升级后的会话
		// 不是一次 HTTP 请求时延，长连接巨值会把 RED 直方图整个污染，故只透传不统计。
		if strings.HasPrefix(c.Request.URL.Path, "/ws/") {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		method := c.Request.Method
		if o.httpRequests != nil {
			o.httpRequests.Add(context.Background(), 1, metric.WithAttributes(
				hMethod(method), hRoute(route), hCode(c.Writer.Status()),
			))
		}
		if o.httpDuration != nil {
			o.httpDuration.Record(context.Background(), time.Since(start).Seconds(),
				metric.WithAttributes(hMethod(method), hRoute(route)))
		}
	}
}

func itoa(v int) string { return strconv.Itoa(v) }
