package model

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// CandidateStatus 表示候选人面试状态的七档状态机。
// NOT_CHECKED_IN → CHECKED_IN_PENDING_ASSIGN → ASSIGNED → IN_PROGRESS → COMPLETED（面试已结束）
// → ADMISSION_PENDING（待录取） → ADMITTED（已录取）
type CandidateStatus string

const (
	StatusNotCheckedIn           CandidateStatus = "NOT_CHECKED_IN"            // 未签到
	StatusCheckedInPendingAssign CandidateStatus = "CHECKED_IN_PENDING_ASSIGN" // 已签到待分配
	StatusAssigned               CandidateStatus = "ASSIGNED"                  // 已分配未开始
	StatusInProgress             CandidateStatus = "IN_PROGRESS"               // 正在进行
	StatusCompleted              CandidateStatus = "COMPLETED"                 // 面试已结束
	StatusAdmissionPending       CandidateStatus = "ADMISSION_PENDING"         // 待录取
	StatusAdmitted               CandidateStatus = "ADMITTED"                  // 已录取
)

// ValidCandidateStatus 返回状态机中全部合法的候选人状态。
func ValidCandidateStatus() []CandidateStatus {
	return []CandidateStatus{
		StatusNotCheckedIn,
		StatusCheckedInPendingAssign,
		StatusAssigned,
		StatusInProgress,
		StatusCompleted,
		StatusAdmissionPending,
		StatusAdmitted,
	}
}

// StatusTransitions 定义了七档状态机的合法转移动图。
// key 为当前状态，value 为该状态下允许跳转的目标状态集合。
var StatusTransitions = map[CandidateStatus][]CandidateStatus{
	StatusNotCheckedIn:           {StatusCheckedInPendingAssign},
	StatusCheckedInPendingAssign: {StatusAssigned},
	StatusAssigned:               {StatusInProgress},
	StatusInProgress:             {StatusCompleted},
	StatusCompleted:              {StatusAdmissionPending},
	StatusAdmissionPending:       {StatusAdmitted},
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
	ID uint64 `gorm:"primaryKey" json:"id"`
	// RoomID 是房间主导字段 rooms.candidate_id 的查询投影，不持久化（gorm:"-"）。
	// 候选人与房间的绑定唯一权威在 rooms.candidate_id（银行叫号：房间占用候选人）。
	RoomID *uint64 `gorm:"-" json:"room_id,omitempty"`
	// StudentNo 学号：候选人的身份键（纯数字、唯一）。
	// 文本列存数字串以保前导零（`00123` ≠ `123`）；校验统一走 ValidateStudentNo，
	// 唯一性由唯一索引兜底。
	StudentNo string          `gorm:"size:64;not null;uniqueIndex" json:"student_no"`
	Name      string          `gorm:"size:128" json:"name"`
	Profile   string          `gorm:"type:text" json:"profile"`
	Status    CandidateStatus `gorm:"size:32;index" json:"status"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// StudentNoMaxLen 学号长度上限（纯数字，1–64 位）。
const StudentNoMaxLen = 64

// ValidateStudentNo 归一化并校验学号：纯数字、1–64 位、不含内部空白。
// 归一化 = 去除首尾空白（含全角空格）+ 全角数字（U+FF10–U+FF19）折为半角；
// 前导零有意义（学号是身份键），归一化不改变数字序列。返回归一化后的学号。
func ValidateStudentNo(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", errors.New("学号不能为空")
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= '０' && r <= '９':
			b.WriteRune(r - '０' + '0')
		default:
			return "", errors.New("学号只能包含数字")
		}
	}
	if out := b.String(); len(out) > StudentNoMaxLen {
		return "", fmt.Errorf("学号长度不能超过 %d 位", StudentNoMaxLen)
	}
	return b.String(), nil
}
