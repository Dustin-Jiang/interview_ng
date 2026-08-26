package model

import "time"

// SystemPhase 系统运行阶段：面试阶段 / 录取阶段。
type SystemPhase string

// 系统阶段常量。
const (
	SystemPhaseInterview SystemPhase = "interview"
	SystemPhaseAdmission SystemPhase = "admission"
)

// SystemStatus 系统状态（单行配置表，ID 恒为 1）：标识当前处于面试阶段还是录取阶段。
type SystemStatus struct {
	ID        uint64      `gorm:"primaryKey" json:"id"`
	Phase     SystemPhase `gorm:"size:16;not null" json:"phase"`
	UpdatedAt time.Time   `json:"updated_at"`
}

// Valid 判断阶段是否为合法取值。
func (p SystemPhase) Valid() bool {
	return p == SystemPhaseInterview || p == SystemPhaseAdmission
}
