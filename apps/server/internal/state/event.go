package state

// EventType 状态变更事件类型。
type EventType string

const (
	EventCandidateSignedIn EventType = "candidate_signed_in" // 候选人签到
	EventCandidateAssigned EventType = "candidate_assigned"  // 分配候选人到房间
	EventRoomPhaseChanged  EventType = "room_phase_changed"  // 房间/候选人阶段变化
	EventMessageAppended   EventType = "message_appended"    // 新聊天消息
	EventMessageUpdated    EventType = "message_updated"     // 消息被编辑（2 分钟内、仅本人）
	EventMessageDeleted    EventType = "message_deleted"     // 消息被撤回（2 分钟内、仅本人）
	// EventMessageReactionsChanged 表情回复增减（载荷为「谁 + 哪个表情 + 加还是撤」的增量，
	// 与观察者无关：计数与「我回没回」由各前端按当前用户自行聚合）。
	EventMessageReactionsChanged EventType = "message_reactions_changed"
	EventMemberJoined      EventType = "member_joined"       // 面试官加入房间
	EventMemberLeft        EventType = "member_left"         // 面试官离开房间

	EventCandidateCreated EventType = "candidate_created" // 新建候选人（管理面）
	EventCandidateUpdated EventType = "candidate_updated" // 编辑候选人资料
	EventCandidateDeleted EventType = "candidate_deleted" // 删除候选人（级联清房删消息）
	EventRoomCreated      EventType = "room_created"      // 新建空房
	EventRoomDeleted      EventType = "room_deleted"      // 删除空房
	EventRoomRenamed      EventType = "room_renamed"      // 房间命名/改名
	EventLeftoverBid      EventType = "leftover_bid"      // 捡漏出价变更（不含金额，跨部门保密）
	EventLeftoverResolved EventType = "leftover_resolved" // 捡漏候选人结算（最高出价录取）
)

// Event 是不可变的状态变更事件，是"系统内部状态唯一性"的对外契约。
// 它被 state 层产出，供 broadcast 层扇出，也用于房间内续传游标对齐。
type Event struct {
	Seq    uint64    `json:"seq"` // 单调递增的全局事件序号（供幂等对齐）
	RoomID uint64    `json:"room_id"`
	Type   EventType `json:"type"`
	Data   any       `json:"data,omitempty"`
	// MsgID 当 Type==EventMessageAppended 时携带消息自增 id，即房间内续传游标。
	MsgID uint64 `json:"msg_id,omitempty"`
}

// CandidateRef 候选人类事件（created/updated/deleted）的载荷。
type CandidateRef struct {
	CandidateID uint64 `json:"candidate_id"`
}

// MessageRef 消息编辑/撤回事件（message_updated / message_deleted）的载荷。
// 字段刻意不带 json tag：与同族的 message_appended 载荷一样按 Go 字段名序列化，
// 前端两处（房间页 / 候选人查看页）都以同名键读取。
type MessageRef struct {
	CandidateID uint64
	MessageID   uint64
	// Content 仅 message_updated 携带（编辑后的正文，已去首尾空白）。
	Content string
}

// ReactionRef 表情回复增减（message_reactions_changed）的载荷：一条增量，与观察者无关。
// 同样按 Go 字段名序列化（与消息族其余事件一致）。
// 带上回复人的展示名与部门（与 message_appended 带 SenderName/SenderDepartment 同理）：
// 「谁回了什么」的明细靠事件就能显示，前端不必为一次点击去查用户表（面试官也没有列用户的权限）。
type ReactionRef struct {
	CandidateID uint64
	MessageID   uint64
	Emoji       string
	// UserID 动作者：前端据此判断「是不是我回的表情」（撤销自己的回复时据此移除）。
	UserID uint64
	// Added true = 加上该表情，false = 撤回该表情。
	Added bool
	// UserName / UserDepartment 回复人的展示名与部门名（无部门为空串；用户已删为空串）。
	UserName       string
	UserDepartment string
}

// RoomRef 房间类事件（created/deleted/renamed）的载荷。
type RoomRef struct {
	RoomID uint64 `json:"room_id"`
}

// CandidateIDOf 从候选人类事件载荷提取候选人 id；载荷缺失或类型不符返回 0。
func CandidateIDOf(ev *Event) uint64 {
	if ev == nil {
		return 0
	}
	if r, ok := ev.Data.(CandidateRef); ok {
		return r.CandidateID
	}
	return 0
}

// LeftoverRef 捡漏类事件（bid/resolved）的载荷。出价事件不携带金额（跨部门保密）；
// 结算事件附 Amount（成交金额，结算后公开）。
type LeftoverRef struct {
	CandidateID  uint64 `json:"candidate_id"`
	DepartmentID uint64 `json:"department_id"`
	Amount       int    `json:"amount,omitempty"`
}

// RoomIDOf 从房间类事件载荷提取房间 id；载荷缺失或类型不符返回 0。
func RoomIDOf(ev *Event) uint64 {
	if ev == nil {
		return 0
	}
	if r, ok := ev.Data.(RoomRef); ok {
		return r.RoomID
	}
	return 0
}
