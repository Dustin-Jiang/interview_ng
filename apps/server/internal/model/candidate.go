package model

import "time"

// CandidateStatus 表示候选人面试状态的五档状态机。
// NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED
type CandidateStatus string

const (
	StatusNotCheckedIn           CandidateStatus = "NOT_CHECKED_IN"            // 未签到
	StatusCheckedInPendingAssign CandidateStatus = "CHECKED_IN_PENDING_ASSIGN" // 已签到待分配
	StatusAssigned               CandidateStatus = "ASSIGNED"                  // 已分配未开始
	StatusInProgress             CandidateStatus = "IN_PROGRESS"               // 正在进行
	StatusCompleted              CandidateStatus = "COMPLETED"                 // 已结束
)

// ValidCandidateStatus 返回状态机中全部合法的候选人状态。
func ValidCandidateStatus() []CandidateStatus {
	return []CandidateStatus{
		StatusNotCheckedIn,
		StatusCheckedInPendingAssign,
		StatusAssigned,
		StatusInProgress,
		StatusCompleted,
	}
}

// StatusTransitions 定义了五档状态机的合法转移动图。
// key 为当前状态，value 为该状态下允许跳转的目标状态集合。
var StatusTransitions = map[CandidateStatus][]CandidateStatus{
	StatusNotCheckedIn:           {StatusCheckedInPendingAssign},
	StatusCheckedInPendingAssign: {StatusAssigned},
	StatusAssigned:               {StatusInProgress},
	StatusInProgress:             {StatusCompleted},
}

// CanTransition 判断 from -> to 是否为合法转移。
func CanTransition(from, to CandidateStatus) bool {
	for _, t := range StatusTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Candidate 面试者（非登录用户，是被面试/被记录的客体）。
type Candidate struct {
	ID        uint64          `gorm:"primaryKey" json:"id"`
	RoomID    *uint64         `gorm:"index" json:"room_id,omitempty"` // 绑定房间(已分配后非空)
	Name      string          `gorm:"size:128" json:"name"`
	Profile   string          `gorm:"type:text" json:"profile"`
	Status    CandidateStatus `gorm:"size:32;index" json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}
