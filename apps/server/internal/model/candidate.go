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

// Interviewing 判断该档位是否处于「面试进行中」（已分配 / 正在进行中）。
// 房间内的消息通道只在此期间开放：面试结档（已完成及其后的待录取 / 已录取）时房间即解绑，
// 面试记录转为只读归档——空房间与已结档的房间都不再接收新消息。
func (s CandidateStatus) Interviewing() bool {
	return s == StatusAssigned || s == StatusInProgress
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
	StudentNo string `gorm:"size:64;not null;uniqueIndex" json:"student_no"`
	Name      string `gorm:"size:128" json:"name"`
	Profile   string `gorm:"type:text" json:"profile"`
	// 报考志愿与联系方式：均为可选资料字段（空白归一化为空串），AutoMigrate 直接加列。
	FirstChoice  string          `gorm:"size:128" json:"first_choice"`  // 第一志愿
	SecondChoice string          `gorm:"size:128" json:"second_choice"` // 第二志愿
	AcceptAdjust bool            `json:"accept_adjust"`                 // 是否接受调剂
	Phone        string          `gorm:"size:32" json:"phone"`          // 手机号
	QQ           string          `gorm:"size:32" json:"qq"`             // QQ 号
	Email        string          `gorm:"size:254" json:"email"`         // 邮箱
	Status       CandidateStatus `gorm:"size:32;index" json:"status"`
	// InterviewStartedAt 当前这次面试的开始时刻：进入「面试中」时打点，离开该状态即置空。
	// 前端计时以此为准——UpdatedAt 会被任何资料编辑刷新，不能当计时起点。
	InterviewStartedAt *time.Time `json:"interview_started_at"`
	// CheckedInAt 本次签到的时刻：签到那一刻打点，只有重置回「未签到」才清空（重新签到重新打点）。
	// 房间「拉取候选人」列表据此先来后到；候场大屏的已签到各档（面试中 / 等待开始 / 等待分配）
	// 也在档内按它排列——所以被拉进房间、开始面试等流转都不清空它。
	// 不能用 UpdatedAt 代替：导入/编辑资料会刷新它，排队顺序会被打乱。
	CheckedInAt *time.Time `json:"checked_in_at"`
	// InterviewRoomID / InterviewRoomName：**这场面试是在哪间房间做的**，在候选人推进到
	// 「面试已结束」的那一刻记录——完成即解绑房间（rooms.candidate_id 置空），不留档就再也查不到。
	// 名字另存快照，房间之后改名或删除仍能回答「当时在哪间」；重复面试（重置后再走一遍）覆盖为最近一场。
	// 房间被删除时 ID 置空（见 DeleteRoom），名字快照保留。
	InterviewRoomID   *uint64   `json:"interview_room_id"`
	InterviewRoomName string    `gorm:"size:128" json:"interview_room_name"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
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
