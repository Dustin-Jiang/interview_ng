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

// CreateCandidate 新建候选人（未签到）。先落库，成功后广播。
func (s *InterviewService) CreateCandidate(ctx context.Context, info state.CandidateInfo) (*state.Event, error) {
	ev, err := s.store.CreateCandidate(ctx, info)
	if err != nil {
		return nil, err
	}
	s.broad.Publish(ev)
	return ev, nil
}

// ImportCandidates 批量导入候选人（管理员数据导入）。先落库（单事务全或无），
// 提交成功后再逐条广播 —— 整批事件与整批数据同生共死。
func (s *InterviewService) ImportCandidates(ctx context.Context, rows []state.CandidateImportRow) (*state.ImportReport, error) {
	report, err := s.store.ImportCandidates(ctx, rows)
	if err != nil {
		return nil, err
	}
	for _, ev := range report.Events {
		s.broad.Publish(ev)
	}
	return report, nil
}

// Publish 直接扇出一个事件（供 handler 在需要时手动广播已落库事件）。
func (s *InterviewService) Publish(ev *state.Event) {
	s.broad.Publish(ev)
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

// AppendCandidateMessage 向候选人面试记录归档补充一条消息（不需要房间与在场成员，
// 面试结档后仍可补充）。先落库后广播；返回事件供 handler 取新记录 id。
func (s *InterviewService) AppendCandidateMessage(ctx context.Context, candidateID, senderID uint64, content string) (*state.Event, error) {
	ev, err := s.store.AppendCandidateMessage(ctx, candidateID, senderID, content)
	if err != nil {
		return nil, err
	}
	s.broad.Publish(ev)
	return ev, nil
}

// EditMessage 编辑自己的面试记录（窗口内）。先落库后广播，变更实时送达房间页与归档页。
func (s *InterviewService) EditMessage(ctx context.Context, candidateID, messageID, editorID uint64, content string) error {
	ev, err := s.store.EditMessage(ctx, candidateID, messageID, editorID, content)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// DeleteMessage 撤回自己的面试记录（窗口内）。先落库后广播。
func (s *InterviewService) DeleteMessage(ctx context.Context, candidateID, messageID, operatorID uint64) error {
	ev, err := s.store.DeleteMessage(ctx, candidateID, messageID, operatorID)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// SetMessageReaction 开关表情回复（幂等）。先落库后广播；状态未变化时 store 返回 nil（无事件）。
func (s *InterviewService) SetMessageReaction(ctx context.Context, candidateID, messageID, userID uint64, emoji string, on bool) error {
	ev, err := s.store.SetMessageReaction(ctx, candidateID, messageID, userID, emoji, on)
	if err != nil {
		return err
	}
	if ev == nil {
		return nil
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

func (s *InterviewService) UpdateUser(ctx context.Context, id uint64, username, name string, departmentID *uint64, roleIDs []uint64) error {
	return s.store.UpdateUser(ctx, id, username, name, departmentID, roleIDs)
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

// ---- 部门管理 ----

func (s *InterviewService) ListDepartments(ctx context.Context) ([]*dsmodel.Department, error) {
	return s.store.ListDepartments(ctx)
}

func (s *InterviewService) CreateDepartment(ctx context.Context, name, desc string, expectedCount int) (uint64, error) {
	return s.store.CreateDepartment(ctx, name, desc, expectedCount)
}

func (s *InterviewService) UpdateDepartment(ctx context.Context, id uint64, name, desc string, expectedCount int) error {
	return s.store.UpdateDepartment(ctx, id, name, desc, expectedCount)
}

func (s *InterviewService) DeleteDepartment(ctx context.Context, id uint64) error {
	return s.store.DeleteDepartment(ctx, id)
}

// ---- 系统状态 ----

func (s *InterviewService) GetSystemStatus(ctx context.Context) (*dsmodel.SystemStatus, error) {
	return s.store.GetSystemStatus(ctx)
}

func (s *InterviewService) SetSystemStatus(ctx context.Context, phase dsmodel.SystemPhase) error {
	return s.store.SetSystemStatus(ctx, phase)
}

// SetBidStep 设置出价步长（管理面板）。
func (s *InterviewService) SetBidStep(ctx context.Context, step int) error {
	return s.store.SetBidStep(ctx, step)
}

// ---- 单点登录（OIDC）配置 ----

// GetOidcConfig 返回 OIDC 配置（含按 position 升序的规则）。
func (s *InterviewService) GetOidcConfig(ctx context.Context) (*dsmodel.OidcConfig, error) {
	return s.store.GetOidcConfig(ctx)
}

// SetOidcConfig 覆盖保存 OIDC 配置与规则（clientSecret 为 nil 表示保持原密钥）。
func (s *InterviewService) SetOidcConfig(ctx context.Context, cfg *dsmodel.OidcConfig, clientSecret *string) error {
	return s.store.SetOidcConfig(ctx, cfg, clientSecret)
}

// GetObservabilityConfig 返回可观测性（OTLP → GreptimeDB）配置。
func (s *InterviewService) GetObservabilityConfig(ctx context.Context) (*dsmodel.ObservabilityConfig, error) {
	return s.store.GetObservabilityConfig(ctx)
}

// SetObservabilityConfig 覆盖保存可观测性配置（password 三段语义见 state 接口注释）。
func (s *InterviewService) SetObservabilityConfig(ctx context.Context, cfg *dsmodel.ObservabilityConfig, password *string) error {
	return s.store.SetObservabilityConfig(ctx, cfg, password)
}

// ---- 候选人管理 ----

// UpdateCandidate 编辑候选人资料字段（学号/姓名/简介/志愿/联系方式）。先落库后广播。
func (s *InterviewService) UpdateCandidate(ctx context.Context, id uint64, info state.CandidateInfo) error {
	ev, err := s.store.UpdateCandidate(ctx, id, info)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// UpdateCandidatePreferences 修改候选人志愿与调剂（独立小权限，不触碰其他资料）。先落库后广播。
func (s *InterviewService) UpdateCandidatePreferences(ctx context.Context, id uint64, prefs state.CandidatePreferences) error {
	ev, err := s.store.UpdateCandidatePreferences(ctx, id, prefs)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// DeleteCandidate 删除候选人（级联删消息、解绑房间）。先落库后广播。
func (s *InterviewService) DeleteCandidate(ctx context.Context, id uint64) error {
	ev, err := s.store.DeleteCandidate(ctx, id)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// ListCandidateAdmissions 返回部门对候选人的录取决定（departmentID 为 nil 时跨部门查看）。
func (s *InterviewService) ListCandidateAdmissions(ctx context.Context, departmentID *uint64) ([]*dsmodel.CandidateAdmission, error) {
	return s.store.ListCandidateAdmissions(ctx, departmentID)
}

// UpsertCandidateAdmission 记录/更新某部门对候选人的录取决定。
func (s *InterviewService) UpsertCandidateAdmission(ctx context.Context, candidateID, departmentID uint64, status dsmodel.AdmissionStatus) error {
	return s.store.UpsertCandidateAdmission(ctx, candidateID, departmentID, status)
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

// CreateRoom 新建房间（name 可选，空串=未命名）。先落库，成功后广播。
func (s *InterviewService) CreateRoom(ctx context.Context, name string) (*state.Event, error) {
	ev, err := s.store.CreateRoom(ctx, name)
	if err != nil {
		return nil, err
	}
	s.broad.Publish(ev)
	return ev, nil
}

// RenameRoom 修改房间名（空串=清除命名）。先落库，成功后广播。
func (s *InterviewService) RenameRoom(ctx context.Context, id uint64, name string) error {
	ev, err := s.store.RenameRoom(ctx, id, name)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// DeleteRoom 删除空房。先落库，成功后广播。
func (s *InterviewService) DeleteRoom(ctx context.Context, id uint64) error {
	ev, err := s.store.DeleteRoom(ctx, id)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
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

// ---- 捡漏竞拍 ----

// LeftoverFinalResults 结算阶段最终录取结果（只读计算；默认仅本部门/已成交可见，exposeAll 全量）。
func (s *InterviewService) LeftoverFinalResults(ctx context.Context, myDepartmentID *uint64, exposeAll bool) ([]*dsmodel.LeftoverFinalResult, error) {
	return s.store.LeftoverFinalResults(ctx, myDepartmentID, exposeAll)
}

// LeftoverOverview 捡漏总览（exposeAll=true 时全部门 spent/remaining 公开，管理端用）。
func (s *InterviewService) LeftoverOverview(ctx context.Context, myDepartmentID *uint64, exposeAll bool) (*dsmodel.LeftoverOverview, error) {
	return s.store.LeftoverOverview(ctx, myDepartmentID, exposeAll)
}

// ListLeftoverBids 返回出价列表（departmentID 为 nil 时跨部门，管理端查看；否则仅本部门）。
func (s *InterviewService) ListLeftoverBids(ctx context.Context, departmentID *uint64) ([]*dsmodel.Bid, error) {
	return s.store.ListLeftoverBids(ctx, departmentID)
}

// UpsertLeftoverBid 记录/覆盖本部门出价。先落库后广播。
func (s *InterviewService) UpsertLeftoverBid(ctx context.Context, candidateID, departmentID uint64, amount int) error {
	ev, err := s.store.UpsertLeftoverBid(ctx, candidateID, departmentID, amount)
	if err != nil {
		return err
	}
	s.broad.Publish(ev)
	return nil
}

// ListLeftoverResults 返回已结算候选人的赢家与成交金额。
func (s *InterviewService) ListLeftoverResults(ctx context.Context) ([]*dsmodel.LeftoverResult, error) {
	return s.store.ListLeftoverResults(ctx)
}
