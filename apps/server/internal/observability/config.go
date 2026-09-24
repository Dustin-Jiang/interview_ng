package observability

import (
	"encoding/base64"
	"net/url"
	"os"
	"strings"
)

// Config 可观测性配置，统一从 OTEL_EXPORTER_OTLP_* 环境变量读取。
// Endpoint 为空 = 观测关闭（零开销分支：meter/logger provider 保持 noop）。
type Config struct {
	Endpoint    string // OTLP 服务地址，形如 http://greptime:4000
	Database    string // 目标库名（GreptimeDB 的 X-Greptime-DB-Name）
	Username    string // Basic 认证用户名
	Password    string // Basic 认证密码（可为空）
	ServiceName string
}

// FromEnv 从环境变量读配置；缺省值见各字段注释。
func FromEnv() Config {
	return Config{
		Endpoint:    strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
		Database:    envOr("OTEL_EXPORTER_OTLP_DATABASE", "interview_ng"),
		Username:    envOr("OTEL_EXPORTER_OTLP_USERNAME", "greptime"),
		Password:    os.Getenv("OTEL_EXPORTER_OTLP_PASSWORD"),
		ServiceName: envOr("OTEL_SERVICE_NAME", "interview_ng"),
	}
}

// Enabled 观测是否已配置。
func (c Config) Enabled() bool { return c.Endpoint != "" }

// headers 推送到 GreptimeDB 所需的鉴权与库名头。
func (c Config) headers() map[string]string {
	return map[string]string{
		"X-Greptime-DB-Name": c.Database,
		"Authorization":      "Basic " + base64.StdEncoding.EncodeToString([]byte(c.Username+":"+c.Password)),
	}
}

// splitEndpoint 把 http(s)://host:port[/base] 拆成 exporter 需要的三段：
// host（含端口，不带协议）、是否 http（OTLP exporter 默认走 https，显式 http 才用非 TLS）、
// 路径前缀（GreptimeDB 的 OTLP 前缀固定为 /v1/otlp；u.Path 是反代基路径时有值）。
func (c Config) splitEndpoint() (host string, insecure bool, pathPrefix string, ok bool) {
	u, err := url.Parse(c.Endpoint)
	if err != nil || u.Host == "" {
		return "", false, "", false
	}
	if u.Path != "" && u.Path != "/" {
		pathPrefix = strings.TrimRight(u.Path, "/")
	}
	return u.Host, strings.EqualFold(u.Scheme, "http"), pathPrefix, true
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
