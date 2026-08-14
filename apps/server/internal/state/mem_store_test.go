package state_test

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/state"
)

func newTestStore(t *testing.T) state.StateStore {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&dsmodel.User{}, &dsmodel.Candidate{}, &dsmodel.Room{},
		&dsmodel.RoomMember{}, &dsmodel.Message{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return state.NewMemStateStore(db)
}

func TestStateMachineTransitions(t *testing.T) {
	cases := []struct {
		from, to dsmodel.CandidateStatus
		ok       bool
	}{
		{dsmodel.StatusNotCheckedIn, dsmodel.StatusCheckedInPendingAssign, true},
		{dsmodel.StatusCheckedInPendingAssign, dsmodel.StatusAssigned, true},
		{dsmodel.StatusAssigned, dsmodel.StatusInProgress, true},
		{dsmodel.StatusInProgress, dsmodel.StatusCompleted, true},

		{dsmodel.StatusNotCheckedIn, dsmodel.StatusCompleted, false},
		{dsmodel.StatusAssigned, dsmodel.StatusCompleted, false},
		{dsmodel.StatusInProgress, dsmodel.StatusAssigned, false},
		{dsmodel.StatusCompleted, dsmodel.StatusInProgress, false},
	}
	for _, c := range cases {
		if got := dsmodel.CanTransition(c.from, c.to); got != c.ok {
			t.Errorf("CanTransition(%s->%s)=%v, want %v", c.from, c.to, got, c.ok)
		}
	}
}

func TestLifecycleThroughStore(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// 新建候选人
	id, err := st.CreateCandidate(ctx, "张三", "后端岗")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	c, _ := st.GetCandidate(ctx, id)
	if c.Status != dsmodel.StatusNotCheckedIn {
		t.Fatalf("initial status=%s", c.Status)
	}

	// 签到
	if _, err := st.CheckIn(ctx, id); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	// 非法重复签到应拒绝
	if _, err := st.CheckIn(ctx, id); err == nil {
		t.Fatalf("expected duplicate checkin error")
	}

	// 分配（新建房间）
	ev, roomID, err := st.AssignCandidate(ctx, id, 0)
	if err != nil {
		t.Fatalf("assign: %v", err)
	}
	if roomID == 0 {
		t.Fatalf("roomID=0")
	}
	if ev.RoomID != roomID {
		t.Fatalf("event room mismatch")
	}

	// 进入进行中
	if _, err := st.MovePhase(ctx, roomID, 0, dsmodel.StatusInProgress); err != nil {
		t.Fatalf("move: %v", err)
	}
	// 非法跳转到 COMPLETED 前的状态应拒绝（IN_PROGRESS -> COMPLETED 合法，这里测 ASSIGNED 状态下的非法）
	c, _ = st.GetCandidate(ctx, id)
	if c.Status != dsmodel.StatusInProgress {
		t.Fatalf("status=%s", c.Status)
	}

	// 加入房间并发消息（需先成为成员才能发）
	_, _, err = st.JoinRoom(ctx, roomID, 1)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	// 已分配候选人不允许重复分配
	if _, _, err := st.AssignCandidate(ctx, id, 0); err == nil {
		t.Fatalf("expected duplicate assign error")
	}

	mev, err := st.AppendMessage(ctx, roomID, 1, "hello")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if mev.MsgID == 0 {
		t.Fatalf("msg id=0")
	}

	// 增量续传：id after MsgID-1 应能取到该消息
	msgs, _ := st.ListMessagesAfter(ctx, roomID, mev.MsgID-1)
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("messages after not found: %+v", msgs)
	}
}

func TestSubscribeDeliversEvents(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id, _ := st.CreateCandidate(ctx, "李四", "前端")
	_, _ = st.CheckIn(ctx, id)
	_, roomID, _ := st.AssignCandidate(ctx, id, 0)

	ch, cancel := st.Subscribe(roomID, 0)
	defer cancel()

	_, _, _ = st.JoinRoom(ctx, roomID, 7)
	_, err := st.AppendMessage(ctx, roomID, 7, "hi")
	if err != nil {
		t.Fatalf("append: %v", err)
	}

	seen := map[state.EventType]bool{}
	for ev := range ch {
		seen[ev.Type] = true
		if ev.Type == state.EventMessageAppended && ev.MsgID > 0 {
			break
		}
	}
	if !seen[state.EventMemberJoined] || !seen[state.EventMessageAppended] {
		t.Fatalf("missing events: %+v", seen)
	}
}
