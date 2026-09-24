package state_test

import (
	"context"
	"fmt"
	"sort"
	"testing"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/state"
)

// TestAdjustWaitingPriority 候场队列手动调序：
//  1. 默认（无显式序号）档内按 checked_in_at 先来后到；
//  2. up/down 把自己与相邻一位交换；首次调序把整档固化成 1..n；
//  3. 档首 up / 档尾 down 是幂等 no-op（moved=false、不写库、不广播）；
//  4. 只有「已签到待分配」档可调序，其余档位 invalid_status。
func TestAdjustWaitingPriority(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	a := mustCreateCandidateWithNo(ctx, st, "0100", "甲", "")
	b := mustCreateCandidateWithNo(ctx, st, "0101", "乙", "")
	c := mustCreateCandidateWithNo(ctx, st, "0102", "丙", "")
	for _, id := range []uint64{a, b, c} {
		checkin(t, st, id)
	}

	// 乙提前一位：默认顺序 [甲, 乙, 丙]（签到顺序）→ [乙, 甲, 丙]，整档固化成显式序。
	moved, ev, err := st.AdjustWaitingPriority(ctx, b, "up")
	if err != nil || !moved || ev == nil {
		t.Fatalf("first up: moved=%v ev=%v err=%v", moved, ev, err)
	}
	if ev.Type != state.EventCandidatePriorityChanged {
		t.Fatalf("调序事件类型应为 candidate_priority_changed，实际 %s", ev.Type)
	}
	expectPriority(t, st, map[uint64]int64{b: 1, a: 2, c: 3})

	// 丙（第三位）提前：交换丙与甲的显式序号 → 丙=2，甲=3。
	if moved, _, err := st.AdjustWaitingPriority(ctx, c, "up"); err != nil || !moved {
		t.Fatalf("second up: moved=%v err=%v", moved, err)
	}
	expectPriority(t, st, map[uint64]int64{b: 1, c: 2, a: 3})

	// 边界幂等：档首 up / 档尾 down 不写库、不广播。
	if moved, ev, err := st.AdjustWaitingPriority(ctx, b, "up"); err != nil || moved || ev != nil {
		t.Fatalf("档首 up 应为幂等 no-op: moved=%v ev=%v err=%v", moved, ev, err)
	}
	if moved, ev, err := st.AdjustWaitingPriority(ctx, a, "down"); err != nil || moved || ev != nil {
		t.Fatalf("档尾 down 应为幂等 no-op: moved=%v ev=%v err=%v", moved, ev, err)
	}
	expectPriority(t, st, map[uint64]int64{b: 1, c: 2, a: 3})

	// 非边界 down 后 up 恢复原序。
	if moved, _, err := st.AdjustWaitingPriority(ctx, b, "down"); err != nil || !moved {
		t.Fatalf("非边界 down: moved=%v err=%v", moved, err)
	}
	expectPriority(t, st, map[uint64]int64{c: 1, b: 2, a: 3})
	if moved, _, err := st.AdjustWaitingPriority(ctx, b, "up"); err != nil || !moved {
		t.Fatalf("down 后 up: moved=%v err=%v", moved, err)
	}
	expectPriority(t, st, map[uint64]int64{b: 1, c: 2, a: 3})
}

// TestAdjustWaitingPriorityOnlyQueueTier 只有「已签到待分配」可调序：
// 未签到（没到队）、已分配/面试中（顺序由面试进程决定）一律 invalid_status，
// 否则会为了一个没有读取方的字段写入整档。
func TestAdjustWaitingPriorityOnlyQueueTier(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	unchecked := mustCreateCandidateWithNo(ctx, st, "0110", "未签到", "")
	if _, _, err := st.AdjustWaitingPriority(ctx, unchecked, "up"); !isCode(err, "invalid_status") {
		t.Fatalf("未签到档应 invalid_status，实际 %v", err)
	}

	queued := mustCreateCandidateWithNo(ctx, st, "0111", "待分配", "")
	checkin(t, st, queued)
	if _, _, err := st.AdjustWaitingPriority(ctx, queued, "up"); err != nil {
		t.Fatalf("唯一在队者 up 应幂等 no-op，实际 %v", err)
	}

	assigned := mustCreateCandidateWithNo(ctx, st, "0112", "已分配", "")
	checkin(t, st, assigned)
	if _, err := st.PullCandidate(ctx, mustCreateRoom(ctx, st), assigned); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, _, err := st.AdjustWaitingPriority(ctx, assigned, "up"); !isCode(err, "invalid_status") {
		t.Fatalf("已分配档应 invalid_status，实际 %v", err)
	}
}

// TestWaitingPriorityClearedWithQueueSession 手动序号属于「排队会话」：
// 重新签到（新会话）与重置回未签到都要清空，离开排队体系后不再无声插队。
func TestWaitingPriorityClearedWithQueueSession(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	a := mustCreateCandidateWithNo(ctx, st, "0120", "甲", "")
	b := mustCreateCandidateWithNo(ctx, st, "0121", "乙", "")
	checkin(t, st, a)
	checkin(t, st, b)
	// 乙插到甲前 → 整档固化（乙=1，甲=2）。
	if moved, _, err := st.AdjustWaitingPriority(ctx, b, "up"); err != nil || !moved {
		t.Fatalf("up: moved=%v err=%v", moved, err)
	}
	expectPriority(t, st, map[uint64]int64{b: 1, a: 2})

	// 重置回「未签到」：序号随签到时刻一起清空。
	if _, err := st.ResetCandidateStatus(ctx, b, dsmodel.StatusNotCheckedIn); err != nil {
		t.Fatalf("reset: %v", err)
	}
	expectPriorityNil(t, st, b)

	// 重新签到 = 新会话 → 回到队尾（甲先到，甲在前）。
	checkin(t, st, b)
	expectPriorityNil(t, st, b)
	if got := queueOrder(t, st); len(got) != 2 || got[0] != a || got[1] != b {
		t.Fatalf("重新签到应回队尾，实际顺序 %v", got)
	}
}

// checkin 签到并断言成功。
func checkin(t *testing.T, st state.StateStore, id uint64) {
	t.Helper()
	if _, err := st.CheckIn(context.Background(), id); err != nil {
		t.Fatalf("checkin %d: %v", id, err)
	}
}

// isCode 判定领域错误是否带指定机器码。
func isCode(err error, code string) bool {
	e, ok := err.(*state.Error)
	return ok && e.Code == code
}

// queueOrder 返回候场队列（已签到待分配档）的显示顺序，判据与后端排序同一份 compareWaiting：
// 显式序号升序（null 最后）→ 签到时刻 → 添加顺序。
func queueOrder(t *testing.T, st state.StateStore) []uint64 {
	t.Helper()
	list, err := st.ListCandidates(context.Background(), dsmodel.StatusCheckedInPendingAssign, "", 100, 0)
	if err != nil {
		t.Fatalf("list candidates: %v", err)
	}
	sort.Slice(list, func(i, j int) bool {
		// 与后端调序同一份口径：先看显式序号，再看签到时刻，最后看 id。
		return orderKey(list[i]) < orderKey(list[j])
	})
	var out []uint64
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

// orderKey 把「显式序号（null 最后）+ 签到时刻」压成一个可比较的排序键。
func orderKey(c *dsmodel.Candidate) string {
	rank := int64(1) << 62
	if c.WaitingPriority != nil {
		rank = *c.WaitingPriority
	}
	at := int64(1) << 62
	if c.CheckedInAt != nil {
		at = c.CheckedInAt.UnixNano()
	}
	return fmt.Sprintf("%019d:%019d", rank, at)
}

// expectPriority 断言各候选人的显式序号（调序成果）。
func expectPriority(t *testing.T, st state.StateStore, want map[uint64]int64) {
	t.Helper()
	for id, p := range want {
		c := mustGetCandidate(t, st, id)
		if c.WaitingPriority == nil || *c.WaitingPriority != p {
			t.Fatalf("id=%d 期望序号 %d，实际 %v", id, p, c.WaitingPriority)
		}
	}
}

// expectPriorityNil 断言候选人没有显式序号（回到默认先来后到）。
func expectPriorityNil(t *testing.T, st state.StateStore, id uint64) {
	t.Helper()
	if c := mustGetCandidate(t, st, id); c.WaitingPriority != nil {
		t.Fatalf("id=%d 期望无显式序号，实际 %v", id, *c.WaitingPriority)
	}
}

func mustGetCandidate(t *testing.T, st state.StateStore, id uint64) *dsmodel.Candidate {
	t.Helper()
	c, err := st.GetCandidate(context.Background(), id)
	if err != nil {
		t.Fatalf("GetCandidate %d: %v", id, err)
	}
	return c
}

// TestListWaitingCandidates 候场大屏名单只含「未定局」的档位：
// 面试已结束（COMPLETED）及其后的录取档（待录取 / 已录取）都不在其中（它们有各自的页面），
// 未签到 / 待分配 / 已分配 / 面试中一律在列；关键词筛选与其它列表同口径。
func TestListWaitingCandidates(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	pending := mustCreateCandidateWithNo(ctx, st, "0130", "待定局", "")
	completed := mustCreateCandidateWithNo(ctx, st, "0131", "面试已结束", "")
	admitted := mustCreateCandidateWithNo(ctx, st, "0132", "已录取", "")

	// 走完整流程把两人推到已定局档（重置到 COMPLETED 需要房间绑定，录取档不需要）。
	for _, id := range []uint64{completed, admitted} {
		checkin(t, st, id)
	}
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, completed); err != nil {
		t.Fatalf("pull completed: %v", err)
	}
	if _, err := st.ResetCandidateStatus(ctx, completed, dsmodel.StatusCompleted); err != nil {
		t.Fatalf("reset completed: %v", err)
	}
	if _, err := st.ResetCandidateStatus(ctx, admitted, dsmodel.StatusAdmitted); err != nil {
		t.Fatalf("reset admitted: %v", err)
	}

	list, err := st.ListWaitingCandidates(ctx, "", 50, 0)
	if err != nil {
		t.Fatalf("list waiting: %v", err)
	}
	got := map[uint64]bool{}
	for _, c := range list {
		got[c.ID] = true
	}
	if !got[pending] {
		t.Fatal("未定局的候选人应在大屏名单里")
	}
	if got[completed] || got[admitted] {
		t.Fatalf("已定局的档位不该出现在大屏名单: completed=%v admitted=%v", got[completed], got[admitted])
	}

	// 关键词筛选照常生效（学号前缀）。
	list, err = st.ListWaitingCandidates(ctx, "0130", 50, 0)
	if err != nil {
		t.Fatalf("list waiting by q: %v", err)
	}
	if len(list) != 1 || list[0].ID != pending {
		t.Fatalf("关键词筛选应只回 %d，实际 %v", pending, list)
	}
}
