package state

// EventType 状态变更事件类型。
type EventType string

const (
	EventCandidateSignedIn EventType = "candidate_signed_in" // 候选人签到
	EventCandidateAssigned EventType = "candidate_assigned"  // 分配候选人到房间
	EventRoomPhaseChanged  EventType = "room_phase_changed"  // 房间/候选人阶段变化
	EventMessageAppended   EventType = "message_appended"    // 新聊天消息
	EventMemberJoined      EventType = "member_joined"       // 面试官加入房间
	EventMemberLeft        EventType = "member_left"         // 面试官离开房间
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
