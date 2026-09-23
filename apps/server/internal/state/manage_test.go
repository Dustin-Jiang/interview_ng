package state_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/state"
)

// TestPullCandidateAssigns 房间内拉取候选人：待分配池才可拉，拉后绑定房间并推进到 ASSIGNED。
func TestPullCandidateAssigns(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candID := mustCreateCandidate(ctx, st, "王五", "后端")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID := mustCreateRoom(ctx, st)
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

// TestCheckInQueueOrder 排队顺序的权威依据 = checked_in_at：进入「已签到待分配」时打点、
// 离开即清空、重新签到重新打点；且资料编辑 / 导入**不**刷新它 —— 房间的「拉取候选人」列表
// 据此先来后到排序，若改用 UpdatedAt，一边排队一边导入就会把顺序打乱。
func TestCheckInQueueOrder(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	first := mustCreateCandidateWithNo(ctx, st, "0090", "先到", "")
	later := mustCreateCandidateWithNo(ctx, st, "0091", "后到", "")
	if c, _ := st.GetCandidate(ctx, first); c.CheckedInAt != nil {
		t.Fatalf("新建（未签到）不该有排队时刻: %v", c.CheckedInAt)
	}

	// 依次签到：先签到者的排队时刻不晚于后签到者
	if _, err := st.CheckIn(ctx, first); err != nil {
		t.Fatalf("checkin first: %v", err)
	}
	if _, err := st.CheckIn(ctx, later); err != nil {
		t.Fatalf("checkin later: %v", err)
	}
	a, _ := st.GetCandidate(ctx, first)
	b, _ := st.GetCandidate(ctx, later)
	if a.CheckedInAt == nil || b.CheckedInAt == nil {
		t.Fatalf("签到应打点: first=%v later=%v", a.CheckedInAt, b.CheckedInAt)
	}
	if b.CheckedInAt.Before(*a.CheckedInAt) {
		t.Fatalf("先签到者不该晚于后签到者: first=%v later=%v", a.CheckedInAt, b.CheckedInAt)
	}

	// 资料编辑与批量导入都不经过状态机打点 → 排队时刻原封不动
	stamped := *a.CheckedInAt
	if _, err := st.UpdateCandidate(ctx, first, info3("0090", "改名", "改简介")); err != nil {
		t.Fatalf("update: %v", err)
	}
	if _, err := st.ImportCandidates(ctx, []state.CandidateImportRow{
		{CandidateInfo: info3("0090", "导入名", "")},
	}); err != nil {
		t.Fatalf("import: %v", err)
	}
	if after, _ := st.GetCandidate(ctx, first); after.CheckedInAt == nil || !after.CheckedInAt.Equal(stamped) {
		t.Fatalf("资料编辑/导入不该刷新排队时刻: %v → %v", stamped, after.CheckedInAt)
	}

	// 被拉进房间 / 开始面试 / 完成：签到时刻都保留 —— 候场大屏的已签到各档要在档内按到达先后排列
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, first); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if c, _ := st.GetCandidate(ctx, first); c.CheckedInAt == nil || !c.CheckedInAt.Equal(stamped) {
		t.Fatalf("被拉进房间不该清空签到时刻: %v", c.CheckedInAt)
	}
	// 误拉后重置回「已签到待分配」：保留原签到时刻，先到的人不该被挪到队尾
	if _, err := st.ResetCandidateStatus(ctx, first, dsmodel.StatusCheckedInPendingAssign); err != nil {
		t.Fatalf("reset to pending: %v", err)
	}
	if c, _ := st.GetCandidate(ctx, first); c.CheckedInAt == nil || !c.CheckedInAt.Equal(stamped) {
		t.Fatalf("重置回待分配不该改写签到时刻: %v", c.CheckedInAt)
	}

	// 只有重置回「未签到」才清空；再签到则重新打点且不早于上一次
	if _, err := st.ResetCandidateStatus(ctx, first, dsmodel.StatusNotCheckedIn); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if c, _ := st.GetCandidate(ctx, first); c.CheckedInAt != nil {
		t.Fatalf("重置回未签到应清空签到时刻: %v", c.CheckedInAt)
	}
	if _, err := st.CheckIn(ctx, first); err != nil {
		t.Fatalf("re-checkin: %v", err)
	}
	if c, _ := st.GetCandidate(ctx, first); c.CheckedInAt == nil || c.CheckedInAt.Before(stamped) {
		t.Fatalf("重新签到应重新打点: %v（旧值 %v）", c.CheckedInAt, stamped)
	}
}

// TestPullCandidateConcurrency 并发拉取同一候选人仅一个成功（状态唯一）。
func TestPullCandidateConcurrency(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candID := mustCreateCandidate(ctx, st, "赵六", "前端")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomA := mustCreateRoom(ctx, st)
	roomB := mustCreateRoom(ctx, st)

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

	candID := mustCreateCandidate(ctx, st, "钱七", "算法")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, candID); err != nil {
		t.Fatalf("pull: %v", err)
	}

	// 前向重置到 COMPLETED：有房应成功，且完成即自动解绑房间
	if _, err := st.ResetCandidateStatus(ctx, candID, dsmodel.StatusCompleted); err != nil {
		t.Fatalf("reset forward: %v", err)
	}
	c, _ := st.GetCandidate(ctx, candID)
	if c.Status != dsmodel.StatusCompleted {
		t.Fatalf("status=%s", c.Status)
	}
	// COMPLETED 一律清房：房间解绑、候选人 room_id 置空（与 MovePhase 完成一致）
	room, _ := st.GetRoom(ctx, roomID)
	if room.CandidateID != nil || c.RoomID != nil {
		t.Fatalf("completed should release room: cand=%+v room=%+v", c, room)
	}

	// 后向重置到 NOT_CHECKED_IN：自动解绑房间
	if _, err := st.ResetCandidateStatus(ctx, candID, dsmodel.StatusNotCheckedIn); err != nil {
		t.Fatalf("reset backward: %v", err)
	}
	c, _ = st.GetCandidate(ctx, candID)
	if c.Status != dsmodel.StatusNotCheckedIn || c.RoomID != nil {
		t.Fatalf("backward reset should unbind: %+v", c)
	}
	room, _ = st.GetRoom(ctx, roomID)
	if room.CandidateID != nil {
		t.Fatalf("room should be empty after unbind")
	}

	// 前向重置到 ASSIGNED：无房应拒绝
	if _, err := st.ResetCandidateStatus(ctx, candID, dsmodel.StatusAssigned); err == nil {
		t.Fatalf("expected no_room error")
	}
}

// TestCompleteCandidateClearsRoom 候选人完成（推进到 COMPLETED）后自动清房，
// 房间成员留守、消息按候选人归档保留，并可继续拉取下一候选人。
func TestCompleteCandidateClearsRoom(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candA := mustCreateCandidate(ctx, st, "甲", "后端")
	if _, err := st.CheckIn(ctx, candA); err != nil {
		t.Fatalf("checkin A: %v", err)
	}
	candB := mustCreateCandidate(ctx, st, "乙", "前端")
	if _, err := st.CheckIn(ctx, candB); err != nil {
		t.Fatalf("checkin B: %v", err)
	}
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, candA); err != nil {
		t.Fatalf("pull A: %v", err)
	}
	if _, _, err := st.JoinRoom(ctx, roomID, 1); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := st.AppendMessage(ctx, roomID, 1, "A 的面试记录"); err != nil {
		t.Fatalf("append: %v", err)
	}

	// 依次推进到 IN_PROGRESS -> COMPLETED
	if _, err := st.MovePhase(ctx, roomID, 1, dsmodel.StatusInProgress); err != nil {
		t.Fatalf("move in_progress: %v", err)
	}
	mev, err := st.MovePhase(ctx, roomID, 1, dsmodel.StatusCompleted)
	if err != nil {
		t.Fatalf("move completed: %v", err)
	}
	if mev.Data == nil {
		t.Fatalf("move completed event missing data")
	}

	// 房间解绑候选人、候选人解绑房间，状态保留 COMPLETED
	room, _ := st.GetRoom(ctx, roomID)
	if room.CandidateID != nil {
		t.Fatalf("room should be released after complete: %+v", room)
	}
	c, _ := st.GetCandidate(ctx, candA)
	if c.Status != dsmodel.StatusCompleted || c.RoomID != nil {
		t.Fatalf("candidate should be COMPLETED & unbound: %+v", c)
	}

	// 消息按候选人归档保留（未级联删除，可从候选人维度完整取回）
	msgs, err := st.ListMessagesAfter(ctx, candA, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 1 || msgs[0].Content != "A 的面试记录" {
		t.Fatalf("candidate A messages not preserved: %+v", msgs)
	}

	// 同一房间可继续拉取下一候选人（完成即清房、成员留守）
	if _, err := st.PullCandidate(ctx, roomID, candB); err != nil {
		t.Fatalf("pull B: %v", err)
	}
	room, _ = st.GetRoom(ctx, roomID)
	if room.CandidateID == nil || *room.CandidateID != candB {
		t.Fatalf("room should bind B: %+v", room)
	}
	// 成员仍在房间（未因清房被移除）
	if len(room.Members) != 1 || room.Members[0].UserID != 1 {
		t.Fatalf("members should remain after complete: %+v", room.Members)
	}
}

// TestDeleteCandidateCascade 删候选人连带删消息并解绑房间。
func TestDeleteCandidateCascade(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	candID := mustCreateCandidate(ctx, st, "孙八", "运维")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, candID); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, _, err := st.JoinRoom(ctx, roomID, 99); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := st.AppendMessage(ctx, roomID, 99, "记录"); err != nil {
		t.Fatalf("append: %v", err)
	}

	if _, err := st.DeleteCandidate(ctx, candID); err != nil {
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

	empty := mustCreateRoom(ctx, st)
	if _, err := st.DeleteRoom(ctx, empty); err != nil {
		t.Fatalf("delete empty room: %v", err)
	}

	roomWithMember := mustCreateRoom(ctx, st)
	if _, _, err := st.JoinRoom(ctx, roomWithMember, 5); err != nil {
		t.Fatalf("join: %v", err)
	}
	if _, err := st.DeleteRoom(ctx, roomWithMember); err == nil {
		t.Fatalf("expected error for room with members")
	}

	candID := mustCreateCandidate(ctx, st, "周九", "测试")
	if _, err := st.CheckIn(ctx, candID); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomWithCand := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomWithCand, candID); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, err := st.DeleteRoom(ctx, roomWithCand); err == nil {
		t.Fatalf("expected error for room with candidate")
	}
}

// TestEmptyRoomRejectsMessage 空房间（无候选人）不能发消息。
func TestEmptyRoomRejectsMessage(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	roomID := mustCreateRoom(ctx, st)
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

// TestDepartmentLifecycle 部门创建/用户归属/删除限制。
func TestDepartmentLifecycle(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	deptID, err := st.CreateDepartment(ctx, "后端组", "负责后端岗位", 12)
	if err != nil {
		t.Fatalf("create department: %v", err)
	}

	// 用户创建时归属部门，GetUser/ListUsers 均带出部门
	uid, err := st.CreateUser(ctx, &dsmodel.User{Username: "u1", Name: "张三", PasswordHash: "x", DepartmentID: &deptID}, nil)
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	u, _ := st.GetUser(ctx, uid)
	if u.Department == nil || u.Department.Name != "后端组" {
		t.Fatalf("department not filled: %+v", u.Department)
	}
	users, _ := st.ListUsers(ctx, "", 50, 0)
	if len(users) != 1 || users[0].Department == nil || users[0].Department.ID != deptID {
		t.Fatalf("list users department: %+v", users)
	}

	// ListDepartments 返回预期人数与面试官数
	depts, _ := st.ListDepartments(ctx)
	if len(depts) != 1 || depts[0].MemberCount != 1 {
		t.Fatalf("department member count: %+v", depts)
	}
	if depts[0].ExpectedCount != 12 {
		t.Fatalf("expected count: got %d, want 12", depts[0].ExpectedCount)
	}

	// 更新预期人数与描述
	if err := st.UpdateDepartment(ctx, deptID, "后端组", "负责后端岗位", 20); err != nil {
		t.Fatalf("update department: %v", err)
	}
	depts, _ = st.ListDepartments(ctx)
	if depts[0].ExpectedCount != 20 {
		t.Fatalf("expected count after update: got %d, want 20", depts[0].ExpectedCount)
	}

	// 删除被引用部门应拒绝
	if err := st.DeleteDepartment(ctx, deptID); err == nil {
		t.Fatalf("expected department_in_use error")
	}

	// 更换部门后原部门可删
	if err := st.UpdateUser(ctx, uid, "u1", "张三", nil, nil); err != nil {
		t.Fatalf("update user: %v", err)
	}
	if err := st.DeleteDepartment(ctx, deptID); err != nil {
		t.Fatalf("delete department: %v", err)
	}

	// 引用不存在的部门创建用户应拒绝
	nonexist := uint64(9999)
	if _, err := st.CreateUser(ctx, &dsmodel.User{Username: "u2", Name: "李四", PasswordHash: "x", DepartmentID: &nonexist}, nil); err == nil {
		t.Fatalf("expected department_not_found error")
	}
}

// TestUpdateUserUsername 修改用户名（登录凭证）：可改、空名拒绝、与它人冲突拒绝。
func TestUpdateUserUsername(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	uidA, _ := st.CreateUser(ctx, &dsmodel.User{Username: "alice", Name: "甲", PasswordHash: "x"}, nil)
	uidB, _ := st.CreateUser(ctx, &dsmodel.User{Username: "bob", Name: "乙", PasswordHash: "x"}, nil)

	// 改为新用户名成功，GetUser 可见
	if err := st.UpdateUser(ctx, uidA, "alice_new", "甲", nil, nil); err != nil {
		t.Fatalf("update username: %v", err)
	}
	u, _ := st.GetUser(ctx, uidA)
	if u.Username != "alice_new" {
		t.Fatalf("username not updated: %s", u.Username)
	}

	// 空名拒绝
	if err := st.UpdateUser(ctx, uidA, "  ", "甲", nil, nil); err == nil {
		t.Fatalf("expected username_required error")
	}

	// 与他人冲突拒绝，且原用户名不变
	if err := st.UpdateUser(ctx, uidA, "bob", "甲", nil, nil); err == nil {
		t.Fatalf("expected username_taken error")
	}
	u, _ = st.GetUser(ctx, uidA)
	if u.Username != "alice_new" {
		t.Fatalf("username should be unchanged on conflict: %s", u.Username)
	}
	_ = uidB
}

// TestSystemStatusPhaseSwitch 系统状态：默认面试阶段，可在面试/录取/捡漏/结算四档间切换，非法阶段拒绝。
func TestSystemStatusPhaseSwitch(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// 默认面试阶段
	s, err := st.GetSystemStatus(ctx)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if s.Phase != dsmodel.SystemPhaseInterview {
		t.Fatalf("default phase=%s", s.Phase)
	}

	// 切换到录取阶段并可读回
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseAdmission); err != nil {
		t.Fatalf("set admission: %v", err)
	}
	s, _ = st.GetSystemStatus(ctx)
	if s.Phase != dsmodel.SystemPhaseAdmission {
		t.Fatalf("phase after set=%s", s.Phase)
	}

	// 切换到捡漏阶段并可读回
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set leftover: %v", err)
	}
	s, _ = st.GetSystemStatus(ctx)
	if s.Phase != dsmodel.SystemPhaseLeftover {
		t.Fatalf("phase after set leftover=%s", s.Phase)
	}

	// 切回面试阶段（双向切换）
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseInterview); err != nil {
		t.Fatalf("set interview: %v", err)
	}
	s, _ = st.GetSystemStatus(ctx)
	if s.Phase != dsmodel.SystemPhaseInterview {
		t.Fatalf("phase after set back=%s", s.Phase)
	}

	// 非法阶段拒绝
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhase("bogus")); err == nil {
		t.Fatalf("expected invalid_phase error")
	}
}

func TestBidStepSetting(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	st0, err := st.GetSystemStatus(ctx)
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	if st0.BidStep != dsmodel.DefaultBidStep {
		t.Fatalf("default bid step=%d, want %d", st0.BidStep, dsmodel.DefaultBidStep)
	}
	if err := st.SetBidStep(ctx, 50); err != nil {
		t.Fatalf("set bid step: %v", err)
	}
	st1, _ := st.GetSystemStatus(ctx)
	if st1.BidStep != 50 {
		t.Fatalf("bid step=%d, want 50", st1.BidStep)
	}
	if err := st.SetBidStep(ctx, 0); err == nil {
		t.Fatalf("expected invalid_bid_step")
	}
}

// TestCandidateAdmissionByDepartment 录取决定按部门分别记录：
// 同一候选人在不同部门可各自决定；按部门过滤正确；非法状态拒绝；删候选人级联清除。
func TestCandidateAdmissionByDepartment(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	deptA, _ := st.CreateDepartment(ctx, "后端组", "", 0)
	deptB, _ := st.CreateDepartment(ctx, "前端组", "", 0)
	candID := mustCreateCandidate(ctx, st, "张三", "后端")

	// 各部门各自记录决定
	if err := st.UpsertCandidateAdmission(ctx, candID, deptA, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("upsert deptA: %v", err)
	}
	if err := st.UpsertCandidateAdmission(ctx, candID, deptB, dsmodel.AdmissionWithdrawn); err != nil {
		t.Fatalf("upsert deptB: %v", err)
	}

	// 跨部门查看返回两条
	all, err := st.ListCandidateAdmissions(ctx, nil)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 records, got %d", len(all))
	}

	// 按部门过滤只回本部门
	onlyA, _ := st.ListCandidateAdmissions(ctx, &deptA)
	if len(onlyA) != 1 || onlyA[0].DepartmentID != deptA || onlyA[0].Status != dsmodel.AdmissionAdmitted {
		t.Fatalf("deptA filter: %+v", onlyA)
	}

	// 同部门幂等更新（改为放弃）
	if err := st.UpsertCandidateAdmission(ctx, candID, deptA, dsmodel.AdmissionWithdrawn); err != nil {
		t.Fatalf("upsert overwrite: %v", err)
	}
	onlyA, _ = st.ListCandidateAdmissions(ctx, &deptA)
	if len(onlyA) != 1 || onlyA[0].Status != dsmodel.AdmissionWithdrawn {
		t.Fatalf("after overwrite: %+v", onlyA)
	}

	// 非法状态拒绝
	if err := st.UpsertCandidateAdmission(ctx, candID, deptA, dsmodel.AdmissionStatus("bogus")); err == nil {
		t.Fatalf("expected invalid_admission_status error")
	}

	// 删候选人级联清除录取决定
	if _, err := st.DeleteCandidate(ctx, candID); err != nil {
		t.Fatalf("delete candidate: %v", err)
	}
	afterDel, _ := st.ListCandidateAdmissions(ctx, nil)
	if len(afterDel) != 0 {
		t.Fatalf("admissions should be cascaded, got %+v", afterDel)
	}
}

// TestRoomNaming 房间命名：创建可带名（裁剪空白落库）；RenameRoom 全量覆盖
// （空串=清除命名）；超长拒绝（room_name_invalid）；不存在房间 not_found；
// 事件为 room_renamed 且载荷带房间 id。
func TestRoomNaming(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	ev, err := st.CreateRoom(ctx, "  面试间A  ")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := state.RoomIDOf(ev)
	r, err := st.GetRoom(ctx, id)
	if err != nil || r.Name != "面试间A" {
		t.Fatalf("创建名应裁剪落库: %+v err=%v", r, err)
	}
	// 缺省创建仍可不命名
	if r2, err := st.GetRoom(ctx, mustCreateRoom(ctx, st)); err != nil || r2.Name != "" {
		t.Fatalf("未命名房间 name 应为空串: %+v err=%v", r2, err)
	}

	if rev, err := st.RenameRoom(ctx, id, "终面间"); err != nil ||
		rev.Type != state.EventRoomRenamed || state.RoomIDOf(rev) != id {
		t.Fatalf("改名事件: %v %+v", err, rev)
	}
	if r, _ = st.GetRoom(ctx, id); r.Name != "终面间" {
		t.Fatalf("改名未生效: %+v", r)
	}
	if _, err := st.RenameRoom(ctx, id, "   "); err != nil {
		t.Fatalf("清除命名: %v", err)
	}
	if r, _ = st.GetRoom(ctx, id); r.Name != "" {
		t.Fatalf("空白应清除命名: %+v", r)
	}

	if _, err := st.RenameRoom(ctx, id, strings.Repeat("名", dsmodel.RoomNameMaxLen+1)); err == nil {
		t.Fatal("超长房间名应拒绝")
	} else if se, ok := err.(*state.Error); !ok || se.Code != "room_name_invalid" {
		t.Fatalf("超长错误码: %v", err)
	}
	if _, err := st.RenameRoom(ctx, 9999, "X"); err != state.ErrNotFound {
		t.Fatalf("不存在房间: got %v want ErrNotFound", err)
	}
}

// 志愿与调剂可独立更新：只改这三列，其他资料与运行态（状态机 / 房间绑定）不受影响，
// 且空值同样覆盖（清空志愿 / 取消调剂）。
func TestUpdateCandidatePreferences(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	ev, err := st.CreateCandidate(ctx, state.CandidateInfo{
		StudentNo: "000123", Name: "志愿", Profile: "简介",
		FirstChoice: "技术部", SecondChoice: "电脑诊所部", AcceptAdjust: true,
		Phone: "13800000000", QQ: "12345", Email: "a@b.c",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := ev.Data.(state.CandidateRef).CandidateID
	if _, err := st.CheckIn(ctx, id); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, id); err != nil {
		t.Fatalf("pull: %v", err)
	}

	upd, err := st.UpdateCandidatePreferences(ctx, id, state.CandidatePreferences{
		FirstChoice:  " 数字媒体中心 ", // 落库前去空白
		SecondChoice: "技术保障中心",
		AcceptAdjust: false, // false 也要覆盖
	})
	if err != nil {
		t.Fatalf("update preferences: %v", err)
	}
	if upd.Type != state.EventCandidateUpdated {
		t.Fatalf("事件类型 = %s，期望 %s", upd.Type, state.EventCandidateUpdated)
	}

	c, err := st.GetCandidate(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c.FirstChoice != "数字媒体中心" || c.SecondChoice != "技术保障中心" || c.AcceptAdjust {
		t.Fatalf("志愿/调剂未更新：%+v", c)
	}
	if c.StudentNo != "000123" || c.Name != "志愿" || c.Profile != "简介" ||
		c.Phone != "13800000000" || c.QQ != "12345" || c.Email != "a@b.c" {
		t.Fatalf("其他资料被改动：%+v", c)
	}
	if c.Status != dsmodel.StatusAssigned {
		t.Fatalf("状态被改动：%s", c.Status)
	}
	room, err := st.GetRoom(ctx, roomID)
	if err != nil || room.CandidateID == nil || *room.CandidateID != id {
		t.Fatalf("房间绑定被改动：%+v err=%v", room, err)
	}

	if _, err := st.UpdateCandidatePreferences(ctx, id, state.CandidatePreferences{}); err != nil {
		t.Fatalf("clear preferences: %v", err)
	}
	if c, _ = st.GetCandidate(ctx, id); c.FirstChoice != "" || c.SecondChoice != "" || c.AcceptAdjust {
		t.Fatalf("清空未生效：%+v", c)
	}

	if _, err := st.UpdateCandidatePreferences(ctx, 999999, state.CandidatePreferences{FirstChoice: "x"}); !errors.Is(err, state.ErrNotFound) {
		t.Fatalf("未知候选人应 ErrNotFound，实得 %v", err)
	}
}
