package model

import "time"

// SystemPhase 系统运行阶段：面试阶段 / 录取阶段 / 捡漏阶段 / 结算阶段。
type SystemPhase string

// 系统阶段常量。
const (
	SystemPhaseInterview  SystemPhase = "interview"
	SystemPhaseAdmission  SystemPhase = "admission"
	SystemPhaseLeftover   SystemPhase = "leftover"   // 捡漏：各部门补录剩余候选人
	SystemPhaseSettlement SystemPhase = "settlement" // 结算：竞拍数据只读，按出价计算最终录取结果
)

// DefaultBidStep 出价步长默认值（上下键调整报价的步进）。
const DefaultBidStep = 10

// SystemStatus 系统状态（单行配置表，ID 恒为 1）：标识当前阶段与出价步长。
type SystemStatus struct {
	ID        uint64      `gorm:"primaryKey" json:"id"`
	Phase     SystemPhase `gorm:"size:16;not null" json:"phase"`
	BidStep   int         `gorm:"not null;default:10" json:"bid_step"` // 出价步长，管理员可改
	UpdatedAt time.Time   `json:"updated_at"`
}

// Valid 判断阶段是否为合法取值。
func (p SystemPhase) Valid() bool {
	return p == SystemPhaseInterview || p == SystemPhaseAdmission ||
		p == SystemPhaseLeftover || p == SystemPhaseSettlement
}
