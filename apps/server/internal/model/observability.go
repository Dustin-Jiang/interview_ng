package model

import "time"

// ObservabilityConfig 是可观测性（OTLP → GreptimeDB）配置单行记录（ID 恒为 1，懒初始化）。
// 管理面板的运行时配置：管理员保存即生效（observability.Apply 重建 provider），
// 无需重启进程；endpoint 为空 / 未启用时观测关闭（noop，零开销）。
// Password 只写不读：界面只显示「是否已配置」，不回收明文（与 OIDC 客户端密钥同口径）。
type ObservabilityConfig struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	Enabled     bool      `gorm:"not null;default:false" json:"enabled"`
	Endpoint    string    `gorm:"size:255;not null;default:''" json:"endpoint"`
	Database    string    `gorm:"size:255;not null;default:'interview_ng'" json:"database"`
	Username    string    `gorm:"size:255;not null;default:'greptime'" json:"username"`
	Password    string    `gorm:"size:255;not null;default:''" json:"-"`
	ServiceName string    `gorm:"size:255;not null;default:'interview_ng'" json:"service_name"`
	UpdatedAt   time.Time `json:"updated_at"`
}
