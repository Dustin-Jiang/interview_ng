package service

import (
	"context"

	dsmodel "interview_ng/internal/model"

	"interview_ng/internal/broadcast"
	"interview_ng/internal/state"
)

// InterviewService 是业务编排层(handler -> service -> state/model)。
// 职责：
//   - 组织单个用例所需的 StateStore 原子操作；
//   - 强制【先落库后广播】顺序：StateStore 返回 Event(已落库) 后才 Publish 到广播层；
//   - 断线续传的装配：连接时拉房间快照 + 消息增量。
//
// handler 层只依赖本服务，不直接触碰 StateStore / model / db。
type InterviewService struct {
	store state.StateStore
	broad *broadcast.Manager
}

// New 构建业务服务。
func New(store state.StateStore, broad *broadcast.Manager) *InterviewService {
	return &InterviewService{store: store, broad: broad}
}

// CheckIn 候选人签到。先落库，成功后广播。
func (s *InterviewService) CheckIn(ctx context.Context, candidateID uint64) error {
	ev, err := s.store.CheckIn(ctx, candidateID)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// CreateCandidate 新建候选人（未签到）。直接落库。
func (s *InterviewService) CreateCandidate(ctx context.Context, name, profile string) (uint64, error) {
	return s.store.CreateCandidate(ctx, name, profile)
}

// Publish 直接扇出一个事件（供 handler 在需要时手动广播已落库事件）。
func (s *InterviewService) Publish(ev *state.Event) {
	s.broad.Publish(ev)
}

// AssignCandidate 分配候选人到房间，返回实际房间 id。先落库后广播。
func (s *InterviewService) AssignCandidate(ctx context.Context, candidateID, roomID uint64) (uint64, error) {
	ev, roomID, err := s.store.AssignCandidate(ctx, candidateID, roomID)
	if err != nil {
		return 0, err
	}
	s.broad.Publish(ev)
	return roomID, nil
}

// MovePhase 推进阶段。先落库后广播。
func (s *InterviewService) MovePhase(ctx context.Context, roomID, operatorID uint64, to dsmodel.CandidateStatus) error {
	ev, err := s.store.MovePhase(ctx, roomID, operatorID, to)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// SendMessage 发送聊天消息。先落库后广播（事件携带 MsgID 供客户端续传对齐）。
func (s *InterviewService) SendMessage(ctx context.Context, roomID, senderID uint64, content string) error {
	ev, err := s.store.AppendMessage(ctx, roomID, senderID, content)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// ---- 用户与角色管理（管理接口不涉及房间事件，直接透传 store） ----

func (s *InterviewService) CreateUser(ctx context.Context, u *dsmodel.User, roleIDs []uint64) (uint64, error) {
	return s.store.CreateUser(ctx, u, roleIDs)
}

func (s *InterviewService) GetUser(ctx context.Context, id uint64) (*dsmodel.User, error) {
	return s.store.GetUser(ctx, id)
}

func (s *InterviewService) ListUsers(ctx context.Context, q string, limit, offset int) ([]*dsmodel.User, error) {
	return s.store.ListUsers(ctx, q, limit, offset)
}

func (s *InterviewService) UpdateUser(ctx context.Context, id uint64, name string, roleIDs []uint64) error {
	return s.store.UpdateUser(ctx, id, name, roleIDs)
}

func (s *InterviewService) SetUserRoles(ctx context.Context, userID uint64, roleIDs []uint64) error {
	return s.store.SetUserRoles(ctx, userID, roleIDs)
}

func (s *InterviewService) ResetUserPassword(ctx context.Context, id uint64, hash string) error {
	return s.store.ResetUserPassword(ctx, id, hash)
}

func (s *InterviewService) DeleteUser(ctx context.Context, id uint64) error {
	return s.store.DeleteUser(ctx, id)
}

func (s *InterviewService) ListRoles(ctx context.Context) ([]*dsmodel.Role, error) {
	return s.store.ListRoles(ctx)
}

func (s *InterviewService) CreateRole(ctx context.Context, name, desc string, perms []string) (uint64, error) {
	return s.store.CreateRole(ctx, name, desc, perms)
}

func (s *InterviewService) UpdateRole(ctx context.Context, id uint64, name, desc string, perms []string) error {
	return s.store.UpdateRole(ctx, id, name, desc, perms)
}

func (s *InterviewService) DeleteRole(ctx context.Context, id uint64) error {
	return s.store.DeleteRole(ctx, id)
}

// ---- 候选人管理 ----

func (s *InterviewService) UpdateCandidate(ctx context.Context, id uint64, name, profile string) error {
	return s.store.UpdateCandidate(ctx, id, name, profile)
}

func (s *InterviewService) DeleteCandidate(ctx context.Context, id uint64) error {
	return s.store.DeleteCandidate(ctx, id)
}

// ResetCandidateStatus 重置候选人状态（含房间绑定联动）。先落库后广播。
func (s *InterviewService) ResetCandidateStatus(ctx context.Context, id uint64, to dsmodel.CandidateStatus) error {
	ev, err := s.store.ResetCandidateStatus(ctx, id, to)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// ---- 房间管理 ----

func (s *InterviewService) CreateRoom(ctx context.Context) (uint64, error) {
	return s.store.CreateRoom(ctx)
}

func (s *InterviewService) DeleteRoom(ctx context.Context, id uint64) error {
	return s.store.DeleteRoom(ctx, id)
}

// PullCandidate 房间内拉取候选人。先落库后广播。
func (s *InterviewService) PullCandidate(ctx context.Context, roomID, candidateID uint64) error {
	ev, err := s.store.PullCandidate(ctx, roomID, candidateID)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// AddRoomMember 把面试官加入房间（复用 JoinRoom 语义：一用户至多一个活跃房间）。先落库后广播。
func (s *InterviewService) AddRoomMember(ctx context.Context, roomID, userID uint64) error {
	_, ev, err := s.store.JoinRoom(ctx, roomID, userID)
	if err != nil {
		return err
	}
	if ev != nil {
		s.broad.Publish(ev)
	}
	return nil
}

// RemoveRoomMember 把面试官移出房间。先落库后广播。
func (s *InterviewService) RemoveRoomMember(ctx context.Context, roomID, userID uint64) error {
	ev, err := s.store.LeaveRoom(ctx, roomID, userID)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}
