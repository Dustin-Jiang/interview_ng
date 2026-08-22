package broadcast_test

import (
	"testing"

	"interview_ng/internal/broadcast"
	"interview_ng/internal/state"
)

// TestPublishGlobalReachesAllSinks 验证全局广播会派发给所有已注册的房间 sink。
func TestPublishGlobalReachesAllSinks(t *testing.T) {
	m := broadcast.New()
	var got []*state.Event

	m.Bind(1, func(ev *state.Event) { got = append(got, ev) })
	m.Bind(2, func(ev *state.Event) { got = append(got, ev) })

	ev := &state.Event{Seq: 7, RoomID: 0, Type: state.EventCandidateSignedIn}
	m.PublishGlobal(ev)

	if len(got) != 2 {
		t.Fatalf("expected 2 deliveries, got %d", len(got))
	}
	for _, g := range got {
		if g != ev {
			t.Fatalf("mismatched event delivered: %+v", g)
		}
	}
}

// TestPublishRoomZeroGoesGlobal 验证 Publish 在 RoomID==0 时走全局广播。
func TestPublishRoomZeroGoesGlobal(t *testing.T) {
	m := broadcast.New()
	var globalCount, room1Count int

	m.Bind(1, func(ev *state.Event) { room1Count++ })
	m.Bind(2, func(ev *state.Event) {
		// 房间 1 的 sink 也会收到（全局）
		globalCount++
	})

	m.Publish(&state.Event{RoomID: 0, Type: state.EventCandidateSignedIn})
	if globalCount == 0 || room1Count == 0 {
		t.Fatalf("RoomID==0 event should fan out globally: room1=%d delivery2=%d", room1Count, globalCount)
	}
}

// TestPublishScopedOnlyTargetRoom 验证普通房间事件只派发给目标房间 sink。
func TestPublishScopedOnlyTargetRoom(t *testing.T) {
	m := broadcast.New()
	sum := 0

	m.Bind(5, func(ev *state.Event) { sum += 100 })
	m.Bind(9, func(ev *state.Event) { sum += 1 })

	m.Publish(&state.Event{RoomID: 5, Type: state.EventMessageAppended})
	if sum != 100 {
		t.Fatalf("expected only room 5 sink called, sum=%d", sum)
	}
}
