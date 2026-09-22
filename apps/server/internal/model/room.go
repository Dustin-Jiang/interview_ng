package model

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Room 面试房间：独立于候选人的物理会议室记录。
//   - 房间可先于候选人存在（candidate_id 可空）；候选人状态为唯一权威，
//     房间自身的"状态"不在库中存储，而是查询时对绑定候选人状态的聚合投影。
//   - 无"主持人"概念：房间内所有面试官地位平等，成员即可推进阶段。
type Room struct {
	ID uint64 `gorm:"primaryKey" json:"id"`
	// Name 房间名：可选别名（空串=未命名，UI 回退显示「房间 #id」）；不要求唯一。
	Name        string       `gorm:"size:128" json:"name"`
	CandidateID *uint64      `gorm:"index" json:"candidate_id,omitempty"` // 可空：未绑定候选人
	Candidate   *Candidate   `gorm:"foreignKey:CandidateID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"candidate,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Members     []RoomMember `gorm:"foreignKey:RoomID" json:"members,omitempty"`
}

// RoomNameMaxLen 房间名长度上限（按字符计）。
const RoomNameMaxLen = 64

// ValidateRoomName 归一化并校验房间名：去首尾空白；允许为空（未命名）；非空时不超上限。
func ValidateRoomName(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if utf8.RuneCountInString(s) > RoomNameMaxLen {
		return "", fmt.Errorf("房间名不能超过 %d 个字符", RoomNameMaxLen)
	}
	return s, nil
}
