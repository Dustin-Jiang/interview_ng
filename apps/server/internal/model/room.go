package model

import "time"

// Room 面试房间：一对一候选人，多面试官对一个候选人。
// 房间自身的"阶段"不单独存储 —— 直接以候选人的 Status 为准（candidates.status），
// 避免出现"房间阶段"与"候选人状态"两套真相导致状态不一致。
// 状态迁移经 state 层 MovePhase 原子操作统一更新 candidates.status。
type Room struct {
	ID                   uint64       `gorm:"primaryKey" json:"id"`
	CandidateID          uint64       `json:"candidate_id"`
	Candidate            *Candidate   `gorm:"foreignKey:CandidateID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"candidate,omitempty"`
	CurrentInterviewerID uint64       `json:"current_interviewer_id,omitempty"` // 当前主持面试的面试官
	CreatedAt            time.Time    `json:"created_at"`
	UpdatedAt            time.Time    `json:"updated_at"`
	Messages             []Message    `gorm:"foreignKey:RoomID" json:"messages,omitempty"`
	Members              []RoomMember `gorm:"foreignKey:RoomID" json:"members,omitempty"`
}
