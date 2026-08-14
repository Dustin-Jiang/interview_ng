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

// SetCurrentInterviewer 切换当前主持面试官。先落库后广播。
func (s *InterviewService) SetCurrentInterviewer(ctx context.Context, roomID, operatorID, newID uint64) error {
	ev, err := s.store.SetCurrentInterviewer(ctx, roomID, operatorID, newID)
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
