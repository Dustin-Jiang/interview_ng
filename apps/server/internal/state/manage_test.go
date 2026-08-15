package state_test

import (
	"context"
	"testing"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/state"
)

// TestPullCandidateAssigns 房间内拉取候选人：待分配池才可拉，拉后绑定房间并推进到 ASSIGNED。
func TestPullCandidateAssigns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candID, _ := st.CreateCandidate(ctx, "王五", "后端")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID, err := st.CreateRoom(ctx)
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	if _, err := st.PullCandidate(ctx, roomID, candID); err != nil {
		t.Fatalf("pull: %v", err)
	}
	c, _ := st.GetCandidate(ctx, candID)
	if c.Status != dsmodel.StatusAssigned || c.RoomID == nil || *c.RoomID != roomID {
		t.Fatalf("candidate not assigned: %+v", c)
	}
	room, _ := st.GetRoom(ctx, roomID)
	if room.Candidate == nil || room.Candidate.ID != candID {
		t.Fatalf("room candidate not bound: %+v", room)
	}
}

// TestPullCandidateConcurrency 并发拉取同一候选人仅一个成功（状态唯一）。
func TestPullCandidateConcurrency(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candID, _ := st.CreateCandidate(ctx, "赵六", "前端")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomA, _ := st.CreateRoom(ctx)
	roomB, _ := st.CreateRoom(ctx)

	type res struct{ err error }
	ch := make(chan res, 2)
	done := make(chan struct{})
	go func() { _, e := st.PullCandidate(ctx, roomA, candID); ch <- res{e}; done <- struct{}{} }()
	go func() { _, e := st.PullCandidate(ctx, roomB, candID); ch <- res{e}; done <- struct{}{} }()
	<-done
	<-done
	close(ch)

	okCount := 0
	for r := range ch {
		if r.err == nil {
			okCount++
		}
	}
	if okCount != 1 {
		t.Fatalf("expected exactly 1 success, got %d", okCount)
	}
	c, _ := st.GetCandidate(ctx, candID)
	if c.RoomID == nil {
		t.Fatalf("candidate should be bound")
	}
	other := roomA
	if *c.RoomID == roomA {
		other = roomB
	}
	otherRoom, _ := st.GetRoom(ctx, other)
	if otherRoom.CandidateID != nil {
		t.Fatalf("other room should stay empty")
	}
}

// TestResetCandidateStatusLinkage 重置状态的后向解绑 / 前向须有房规则。
func TestResetCandidateStatusLinkage(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candID, _ := st.CreateCandidate(ctx, "钱七", "算法")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID, _ := st.CreateRoom(ctx)
	if _, err := st.PullCandidate(ctx, roomID, candID); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// 前向重置到 COMPLETED：有房应成功
	if _, err := st.ResetCandidateStatus(ctx, candID, dsmodel.StatusCompleted); err != nil {
		t.Fatalf("reset forward: %v", err)
	}
	c, _ := st.GetCandidate(ctx, candID)
	if c.Status != dsmodel.StatusCompleted {
		t.Fatalf("status=%s", c.Status)
	}

	// 后向重置到 NOT_CHECKED_IN：自动解绑房间
	if _, err := st.ResetCandidateStatus(ctx, candID, dsmodel.StatusNotCheckedIn); err != nil {
		t.Fatalf("reset backward: %v", err)
	}
	c, _ = st.GetCandidate(ctx, candID)
	if c.Status != dsmodel.StatusNotCheckedIn || c.RoomID != nil {
		t.Fatalf("backward reset should unbind: %+v", c)
	}
	room, _ := st.GetRoom(ctx, roomID)
	if room.CandidateID != nil {
		t.Fatalf("room should be empty after unbind")
	}

	// 前向重置到 ASSIGNED：无房应拒绝
	if _, err := st.ResetCandidateStatus(ctx, candID, dsmodel.StatusAssigned); err == nil {
		t.Fatalf("expected no_room error")
	}
}

// TestDeleteCandidateCascade 删候选人连带删消息并解绑房间。
func TestDeleteCandidateCascade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candID, _ := st.CreateCandidate(ctx, "孙八", "运维")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID, _ := st.CreateRoom(ctx)
	if _, err := st.PullCandidate(ctx, roomID, candID); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, _, err := st.JoinRoom(ctx, roomID, 99); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := st.AppendMessage(ctx, roomID, 99, "记录"); err != nil {
		t.Fatalf("append: %v", err)
	}

	if err := st.DeleteCandidate(ctx, candID); err != nil {
		t.Fatalf("delete candidate: %v", err)
	}
	// 候选人与消息应已删除；房间保留为空记录。
	if _, err := st.GetCandidate(ctx, candID); err != state.ErrNotFound {
		t.Fatalf("candidate should be gone, got %v", err)
	}
	msgs, _ := st.ListMessagesAfter(ctx, candID, 0)
	if len(msgs) != 0 {
		t.Fatalf("messages should be cascaded")
	}
	room, err := st.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("room should persist: %v", err)
	}
	if room.CandidateID != nil {
		t.Fatalf("room should be unbound")
	}
}

// TestRoomDeleteRules 空房可删；绑定候选人或有成员不可删。
func TestRoomDeleteRules(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	empty, _ := st.CreateRoom(ctx)
	if err := st.DeleteRoom(ctx, empty); err != nil {
		t.Fatalf("delete empty room: %v", err)
	}

	roomWithMember, _ := st.CreateRoom(ctx)
	if _, _, err := st.JoinRoom(ctx, roomWithMember, 5); err != nil {
		t.Fatalf("join: %v", err)
	}
	if err := st.DeleteRoom(ctx, roomWithMember); err == nil {
		t.Fatalf("expected error for room with members")
	}

	candID, _ := st.CreateCandidate(ctx, "周九", "测试")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomWithCand, _ := st.CreateRoom(ctx)
	if _, err := st.PullCandidate(ctx, roomWithCand, candID); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if err := st.DeleteRoom(ctx, roomWithCand); err == nil {
		t.Fatalf("expected error for room with candidate")
	}
}

// TestEmptyRoomRejectsMessage 空房间（无候选人）不能发消息。
func TestEmptyRoomRejectsMessage(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	roomID, _ := st.CreateRoom(ctx)
	if _, _, err := st.JoinRoom(ctx, roomID, 1); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := st.AppendMessage(ctx, roomID, 1, "hi"); err == nil {
		t.Fatalf("expected error for empty room message")
	}
}

// TestUserRoleLifecycle 用户创建/角色分配/改密版本号递增。
func TestUserRoleLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	roleID, err := st.CreateRole(ctx, "auditor", "审计", []string{dsmodel.PermRoomsView})
	if err != nil {
		t.Fatalf("create role: %v", err)
	}
	uid, err := st.CreateUser(ctx, &dsmodel.User{Username: "u1", Name: "审计员", PasswordHash: "x"}, []uint64{roleID})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	u, _ := st.GetUser(ctx, uid)
	if len(u.Roles) != 1 || u.Roles[0].Name != "auditor" {
		t.Fatalf("roles not assigned: %+v", u.Roles)
	}

	// 被引用的角色不可删（此时 role 仍被 u1 使用）
	if err := st.DeleteRole(ctx, roleID); err == nil {
		t.Fatalf("expected role_in_use error")
	}

	if err := st.SetUserRoles(ctx, uid, nil); err != nil {
		t.Fatalf("clear roles: %v", err)
	}
	u, _ = st.GetUser(ctx, uid)
	if len(u.Roles) != 0 {
		t.Fatalf("roles should be cleared")
	}

	// 解除引用后可删
	if err := st.DeleteRole(ctx, roleID); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	// 改密 bump token version
	if err := st.ResetUserPassword(ctx, uid, "h2"); err != nil {
		t.Fatalf("reset password: %v", err)
	}
}
