package state

import (
	"context"

	dsmodel "interview_ng/internal/model"
)

// StateStore 是"系统内部状态唯一性"的唯一权威层。
// 暴露业务级原子操作（Q2=B），返回不可变变更事件（Q3=A3）；
// 当前实现为内存版（Q1=A），接口形态保证未来可替换为 Redis 实现而不动调用方。
//
// 持久化顺序约定：每个写操作必须【先落库成功后】才返回 Event（先落库后广播），
// 保证重连增量一定存在于库中，杜绝"客户端看到但库没有"造成的状态不同步。
type StateStore interface {
	// ---- 读：给瞬时一致快照（Q3=B1） ----

	// GetCandidate 返回候选人当前快照。
	GetCandidate(ctx context.Context, id uint64) (*dsmodel.Candidate, error)
	// GetCandidateByRoom 返回某房间绑定的候选人。
	GetCandidateByRoom(ctx context.Context, roomID uint64) (*dsmodel.Candidate, error)
	// GetRoom 返回房间快照（含成员、候选人）。
	GetRoom(ctx context.Context, roomID uint64) (*dsmodel.Room, error)
	// ListRooms 分页列出房间（面试官浏览；每项附带绑定候选人与消息条数）。
	ListRooms(ctx context.Context, limit, offset int) ([]*dsmodel.Room, error)
	// ListCandidates 分页列出候选人（面试官浏览），可按状态与关键词（姓名/简介）筛选。
	ListCandidates(ctx context.Context, status dsmodel.CandidateStatus, q string, limit, offset int) ([]*dsmodel.Candidate, error)
	// ListMessagesAfter 返回房间内 id>afterID 的消息（断线续传增量）。
	ListMessagesAfter(ctx context.Context, roomID uint64, afterID uint64) ([]*dsmodel.Message, error)

	// ---- 写：原子业务操作（Q2=B），先落库后返回事件 ----

	// CheckIn 候选人签到：NOT_CHECKED_IN -> CHECKED_IN_PENDING_ASSIGN。
	CheckIn(ctx context.Context, candidateID uint64) (*Event, error)
	// CreateCandidate 新建候选人（初始状态 NOT_CHECKED_IN），返回其 id。
	CreateCandidate(ctx context.Context, name, profile string) (uint64, error)
	// AssignCandidate 分配候选人到房间：CHECKED_IN_PENDING_ASSIGN -> ASSIGNED。
	// 返回事件与本房间 id（roomID 为 0 时自动新建）。
	AssignCandidate(ctx context.Context, candidateID, roomID uint64) (*Event, uint64, error)
	// MovePhase 推进阶段：ASSIGNED -> IN_PROGRESS -> COMPLETED。
	// 由当前房间成员调用（无主持人概念，成员即可推进）。
	// 推进到 COMPLETED 自动解绑房间（rooms.candidate_id 与候选人 room_id 置空），
	// 房间转空闲可拉取下一候选人；消息仍按候选人归档保留。
	MovePhase(ctx context.Context, roomID, operatorID uint64, to dsmodel.CandidateStatus) (*Event, error)
	// AppendMessage 在房间内追加一条聊天消息，返回事件(带 MsgID)。
	AppendMessage(ctx context.Context, roomID, senderID uint64, content string) (*Event, error)
	// JoinRoom 面试官加入房间（返回房间快照用于首次同步 + 成员变更事件）。
	JoinRoom(ctx context.Context, roomID, userID uint64) (*dsmodel.Room, *Event, error)
	// LeaveRoom 面试官离开房间（返回最后一个离开者时会额外产出成员变更事件）。
	LeaveRoom(ctx context.Context, roomID, userID uint64) (*Event, error)

	// ---- 用户与角色管理（RBAC，先落库后由调用方重载 RBAC 缓存） ----

	// CreateUser 新建面试官（含角色分配），返回用户 id。
	CreateUser(ctx context.Context, u *dsmodel.User, roleIDs []uint64) (uint64, error)
	// GetUser 返回用户（含角色）。
	GetUser(ctx context.Context, id uint64) (*dsmodel.User, error)
	// ListUsers 分页列出用户（可按 username/name 关键词搜索），每项含角色。
	ListUsers(ctx context.Context, q string, limit, offset int) ([]*dsmodel.User, error)
	// UpdateUser 更新用户显示名与角色分配。
	UpdateUser(ctx context.Context, id uint64, name string, roleIDs []uint64) error
	// SetUserRoles 覆盖用户的角色分配。
	SetUserRoles(ctx context.Context, userID uint64, roleIDs []uint64) error
	// ResetUserPassword 重置用户密码（bump token_version，旧 token 即时失效）。
	ResetUserPassword(ctx context.Context, id uint64, hash string) error
	// DeleteUser 删除面试官（room_members 由 FK CASCADE；消息因归候选人不受影响）。
	DeleteUser(ctx context.Context, id uint64) error

	// ListRoles 列出全部角色（含权限组）。
	ListRoles(ctx context.Context) ([]*dsmodel.Role, error)
	// CreateRole 新建角色（权限组）。
	CreateRole(ctx context.Context, name, desc string, perms []string) (uint64, error)
	// UpdateRole 更新角色名与权限组（权限变更后需 ReloadAll 缓存）。
	UpdateRole(ctx context.Context, id uint64, name, desc string, perms []string) error
	// DeleteRole 删除角色（被用户引用时拒绝）。
	DeleteRole(ctx context.Context, id uint64) error

	// ---- 候选人管理 ----

	// UpdateCandidate 编辑候选人姓名/简介。
	UpdateCandidate(ctx context.Context, id uint64, name, profile string) error
	// DeleteCandidate 删除候选人：连带删其消息档案并解绑房间（房间保留为空记录）。
	DeleteCandidate(ctx context.Context, id uint64) error
	// ResetCandidateStatus 重置候选人到状态机任意档：
	// 向后档（未签到/已签到待分配）自动解绑房间；向前档须已有房间绑定。
	// 向前档目标为 COMPLETED 时与推进路径一致：自动解绑房间（候选人与房间均解除关联）。
	ResetCandidateStatus(ctx context.Context, id uint64, to dsmodel.CandidateStatus) (*Event, error)

	// ---- 房间管理 ----

	// CreateRoom 手动创建空房间（独立物理会议室记录），返回房间 id。
	CreateRoom(ctx context.Context) (uint64, error)
	// DeleteRoom 删除空房间（无候选人绑定、无成员时允许）。
	DeleteRoom(ctx context.Context, id uint64) error
	// PullCandidate 房间内面试官拉取候选人：CHECKED_IN_PENDING_ASSIGN -> ASSIGNED 并绑定房间。
	PullCandidate(ctx context.Context, roomID, candidateID uint64) (*Event, error)

	// ---- 事件游标 & 订阅（重连/广播用） ----

	// Subscribe 订阅某个房间自 startAfterSeq 之后的事件流。
	// 返回接收 channel 与取消函数；客户端据此续传对齐。
	Subscribe(roomID uint64, startAfterSeq uint64) (<-chan *Event, func())
}

// Error 提供带业务语义的状态错误。
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

// 常用状态错误。
var (
	ErrNotFound        = &Error{Code: "not_found", Msg: "resource not found"}
	ErrIllegalStatus   = &Error{Code: "illegal_status", Msg: "illegal status transition"}
	ErrRoomFull        = &Error{Code: "room_full", Msg: "room is full"}
	ErrNotMember       = &Error{Code: "not_member", Msg: "operator is not a room member"}
	ErrAlreadyAssigned = &Error{Code: "already_assigned", Msg: "candidate already assigned"}
	ErrUserInRoom      = &Error{Code: "user_in_room", Msg: "user already in an active room"}
)
