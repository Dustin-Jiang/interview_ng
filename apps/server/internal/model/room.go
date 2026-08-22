package model

import "time"

// Room 面试房间：独立于候选人的物理会议室记录。
//   - 房间可先于候选人存在（candidate_id 可空）；候选人状态为唯一权威，
//     房间自身的"状态"不在库中存储，而是查询时对绑定候选人状态的聚合投影。
//   - 无"主持人"概念：房间内所有面试官地位平等，成员即可推进阶段。
type Room struct {
	ID          uint64       `gorm:"primaryKey" json:"id"`
	CandidateID *uint64      `gorm:"index" json:"candidate_id,omitempty"` // 可空：未绑定候选人
	Candidate   *Candidate   `gorm:"foreignKey:CandidateID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"candidate,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Messages    []Message    `gorm:"-" json:"messages,omitempty"`
	Members     []RoomMember `gorm:"foreignKey:RoomID" json:"members,omitempty"`
}
