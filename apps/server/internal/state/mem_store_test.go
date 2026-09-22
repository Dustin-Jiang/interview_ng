package state_test

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

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
		&dsmodel.Role{}, &dsmodel.RolePermission{}, &dsmodel.UserRole{},
		&dsmodel.Department{}, &dsmodel.SystemStatus{}, &dsmodel.CandidateAdmission{}, &dsmodel.Bid{},
		&dsmodel.OidcConfig{}, &dsmodel.OidcRoleRule{}, &dsmodel.OidcDeptRule{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return state.NewMemStateStore(db)
}

// mustCreateCandidate 创建候选人并返回其 id（从创建事件载荷提取，测试辅助）。
// 学号自动生成（保证唯一且为纯数字）；学号自身的校验规则由专门用例覆盖。
func mustCreateCandidate(ctx context.Context, st state.StateStore, name, profile string) uint64 {
	seq := testStudentNo.Add(1)
	ev, err := st.CreateCandidate(ctx, info3(fmt.Sprintf("%08d", seq), name, profile))
	if err != nil {
		panic(err)
	}
	return state.CandidateIDOf(ev)
}

// testStudentNo 测试用学号自增序号（跨用例唯一，避免唯一索引冲突）。
var testStudentNo atomic.Uint64

// mustCreateRoom 创建空房并返回其 id（测试辅助）。
func mustCreateRoom(ctx context.Context, st state.StateStore) uint64 {
	ev, err := st.CreateRoom(ctx, "")
	if err != nil {
		panic(err)
	}
	return state.RoomIDOf(ev)
}

// mustCompleteCandidate 把候选人推进到 COMPLETED（签到 → 拉房 → 完成），测试辅助。
func mustCompleteCandidate(ctx context.Context, st state.StateStore, id uint64) {
	if _, err := st.CheckIn(ctx, id); err != nil {
		panic(err)
	}
	if _, err := st.PullCandidate(ctx, mustCreateRoom(ctx, st), id); err != nil {
		panic(err)
	}
	if _, err := st.ResetCandidateStatus(ctx, id, dsmodel.StatusCompleted); err != nil {
		panic(err)
	}
}

// drainResolved 排空订阅通道中已投递的 leftover_resolved 事件（按候选人归集）。
// 事件在持锁路径内同步投递，触发结算后即可立即排空。
func drainResolved(ch <-chan *state.Event) map[uint64]state.LeftoverRef {
	out := map[uint64]state.LeftoverRef{}
	for {
		select {
		case ev := <-ch:
			if ev.Type != state.EventLeftoverResolved {
				continue
			}
			if ref, ok := ev.Data.(state.LeftoverRef); ok {
				out[ref.CandidateID] = ref
			}
		default:
			return out
		}
	}
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
	id := mustCreateCandidate(ctx, st, "张三", "后端岗")
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

	// 建房 → 房间内拉取候选人（叫号式）
	roomID := mustCreateRoom(ctx, st)
	ev, err := st.PullCandidate(ctx, roomID, id)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if ev.RoomID != roomID {
		t.Fatalf("event room mismatch")
	}

	// 加入房间并发消息（需先成为成员才能推进阶段/发消息）
	_, _, err = st.JoinRoom(ctx, roomID, 1)
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	// 已入房候选人不允许被再次拉取
	if _, err := st.PullCandidate(ctx, roomID, id); err == nil {
		t.Fatalf("expected duplicate pull error")
	}

	// 进入进行中（成员即可推进，无主持人概念）
	if _, err := st.MovePhase(ctx, roomID, 1, dsmodel.StatusInProgress); err != nil {
		t.Fatalf("move: %v", err)
	}
	// 非法跳转到 COMPLETED 前的状态应拒绝（IN_PROGRESS -> COMPLETED 合法，这里测 ASSIGNED 状态下的非法）
	c, _ = st.GetCandidate(ctx, id)
	if c.Status != dsmodel.StatusInProgress {
		t.Fatalf("status=%s", c.Status)
	}

	mev, err := st.AppendMessage(ctx, roomID, 1, "hello")
	if err != nil {
		t.Fatalf("append: %v", err)
	}
	if mev.MsgID == 0 {
		t.Fatalf("msg id=0")
	}

	// 增量续传：id after MsgID-1 应能取到该消息（消息按候选人归属）
	msgs, _ := st.ListMessagesAfter(ctx, id, mev.MsgID-1)
	if len(msgs) != 1 || msgs[0].Content != "hello" {
		t.Fatalf("messages after not found: %+v", msgs)
	}
}

func TestSubscribeDeliversEvents(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := mustCreateCandidate(ctx, st, "李四", "前端")
	_, _ = st.CheckIn(ctx, id)
	roomID := mustCreateRoom(ctx, st)
	_, _ = st.PullCandidate(ctx, roomID, id)

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

func TestLeftoverBudgetFormula(t *testing.T) {
	cases := []struct{ expected, admitted, want int }{
		{20, 10, 1000}, // (20-10)*100
		{20, 15, 500},  // (20-15)*100 = 500
		{2, 0, 500},    // 200 < 500 → 下限
		{0, 0, 500},
		{20, 21, 500}, // 负数 → 下限
	}
	for _, c := range cases {
		if got := dsmodel.LeftoverBudget(c.expected, c.admitted); got != c.want {
			t.Errorf("LeftoverBudget(%d,%d)=%d, want %d", c.expected, c.admitted, got, c.want)
		}
	}
}

func TestLeftoverBiddingAndResolve(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	deptA, err := st.CreateDepartment(ctx, "部门A", "", 20)
	if err != nil {
		t.Fatalf("create deptA: %v", err)
	}
	deptB, err := st.CreateDepartment(ctx, "部门B", "", 2)
	if err != nil {
		t.Fatalf("create deptB: %v", err)
	}
	c1 := mustCreateCandidate(ctx, st, "甲", "")
	c2 := mustCreateCandidate(ctx, st, "乙", "")
	c3 := mustCreateCandidate(ctx, st, "丙", "") // 无任何出价：进入结算阶段不成交
	mustCompleteCandidate(ctx, st, c3)

	// 非捡漏阶段禁止出价
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptA, 100); err == nil {
		t.Fatalf("expected phase guard error")
	}
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set phase: %v", err)
	}

	// 非法金额（负数）与不存在的候选人/部门
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptA, -1); err == nil {
		t.Fatalf("expected invalid amount error")
	}
	if _, err := st.UpsertLeftoverBid(ctx, 99999, deptA, 100); err == nil {
		t.Fatalf("expected not_found")
	}

	// 出价与改价：A 对 c1 出 600 → 800
	ev, err := st.UpsertLeftoverBid(ctx, c1, deptA, 600)
	if err != nil {
		t.Fatalf("bid: %v", err)
	}
	// 出价事件不得携带金额（跨部门保密）
	if ref, ok := ev.Data.(state.LeftoverRef); !ok || ref.Amount != 0 || ref.DepartmentID != deptA {
		t.Fatalf("bid event payload=%+v", ev.Data)
	}
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptA, 800); err != nil {
		t.Fatalf("rebid: %v", err)
	}
	// B 对 c1 出 700；B 预算 500 → 700 直接超预算拒绝
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptB, 700); err == nil {
		t.Fatalf("expected budget_exceeded")
	}
	// B 对 c1 出 300、c2 出 400：总计 700 > 500 拒绝
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptB, 300); err != nil {
		t.Fatalf("bid B c1: %v", err)
	}
	if _, err := st.UpsertLeftoverBid(ctx, c2, deptB, 400); err == nil {
		t.Fatalf("expected budget_exceeded for c2")
	}
	// B 改价 c2 至 150：总 450 ≤ 500 通过
	if _, err := st.UpsertLeftoverBid(ctx, c2, deptB, 150); err != nil {
		t.Fatalf("rebid B c2: %v", err)
	}

	// 出价仅本部门可见
	bidsA, err := st.ListLeftoverBids(ctx, &deptA)
	if err != nil || len(bidsA) != 1 || bidsA[0].Amount != 800 {
		t.Fatalf("bidsA=%+v err=%v", bidsA, err)
	}
	bidsB, _ := st.ListLeftoverBids(ctx, &deptB)
	if len(bidsB) != 2 {
		t.Fatalf("bidsB=%+v", bidsB)
	}

	// 进入结算阶段：自动按出价结算全部竞拍（最高价部门录取），成交事件带金额
	ch, cancel := st.Subscribe(0, 0)
	defer cancel()
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseSettlement); err != nil {
		t.Fatalf("set settlement: %v", err)
	}
	resolved := drainResolved(ch)
	if ref := resolved[c1]; ref.DepartmentID != deptA || ref.Amount != 800 {
		t.Fatalf("c1 resolved=%+v", ref)
	}
	if ref := resolved[c2]; ref.DepartmentID != deptB || ref.Amount != 150 {
		t.Fatalf("c2 resolved=%+v", ref)
	}
	// 幂等：重复进入结算阶段不产生新的成交事件
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseSettlement); err != nil {
		t.Fatalf("re-enter settlement: %v", err)
	}
	if again := drainResolved(ch); len(again) != 0 {
		t.Fatalf("re-settlement must be a no-op: %+v", again)
	}
	// 结算阶段竞拍只读：已成交候选人不能再出价
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptA, 100); err == nil {
		t.Fatalf("expected not_leftover_phase on bid")
	}

	// 录取决定：c1 → A admitted、B withdrawn
	all, err := st.ListCandidateAdmissions(ctx, nil)
	if err != nil {
		t.Fatalf("list admissions: %v", err)
	}
	byDept := map[uint64]map[uint64]dsmodel.AdmissionStatus{} // candidate -> dept -> status
	for _, a := range all {
		if byDept[a.CandidateID] == nil {
			byDept[a.CandidateID] = map[uint64]dsmodel.AdmissionStatus{}
		}
		byDept[a.CandidateID][a.DepartmentID] = a.Status
	}
	if byDept[c1][deptA] != dsmodel.AdmissionAdmitted || byDept[c1][deptB] != dsmodel.AdmissionWithdrawn {
		t.Fatalf("c1 admissions=%+v", byDept[c1])
	}
	if byDept[c2][deptB] != dsmodel.AdmissionAdmitted {
		t.Fatalf("c2 admissions=%+v", byDept[c2])
	}

	// 结果公开：c1 赢家 A 800
	results, err := st.ListLeftoverResults(ctx)
	if err != nil {
		t.Fatalf("results: %v", err)
	}
	got := map[uint64]dsmodel.LeftoverResult{}
	for _, r := range results {
		got[r.CandidateID] = *r
	}
	if got[c1].DepartmentID != deptA || got[c1].Amount != 800 {
		t.Fatalf("c1 result=%+v", got[c1])
	}
	if got[c2].DepartmentID != deptB || got[c2].Amount != 150 {
		t.Fatalf("c2 result=%+v", got[c2])
	}
	// 无出价候选人 c3：不成交（无结果记录），录取档状态保持待录取
	if _, ok := got[c3]; ok {
		t.Fatalf("c3 must have no leftover result: %+v", got[c3])
	}
	c3got, err := st.GetCandidate(ctx, c3)
	if err != nil || c3got.Status != dsmodel.StatusAdmissionPending {
		t.Fatalf("c3 status=%+v err=%v", c3got, err)
	}

	// 总览：A 已录取 1 人 → 预算 (20-1)*100=1900，已出 800，剩 1100
	ov, err := st.LeftoverOverview(ctx, &deptA, false)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	// A 赢得 c1（出价 800 已随录取释放，不再占用预算）：
	// 已录取 1 人 → 预算 (20-1)*100=1900，已出 0，剩 1900
	if ov.My == nil || ov.My.Budget != 1900 || ov.My.Spent != 0 || ov.My.Remaining != 1900 {
		t.Fatalf("my=%+v", ov.My)
	}
	if ov.Phase != dsmodel.SystemPhaseSettlement {
		t.Fatalf("phase=%s", ov.Phase)
	}
	// 其他部门的 spent/remaining 必须保密（nil）
	for _, d := range ov.Departments {
		if d.ID == deptB && (d.Spent != nil || d.Remaining != nil) {
			t.Fatalf("deptB figures leaked: %+v", d)
		}
		if d.ID == deptA && d.Spent == nil {
			t.Fatalf("own dept figures missing: %+v", d)
		}
	}
	// 无部门用户：my 为 nil
	ovAnon, err := st.LeftoverOverview(ctx, nil, false)
	if err != nil || ovAnon.My != nil {
		t.Fatalf("anon my=%+v err=%v", ovAnon.My, err)
	}

	// 管理端（exposeAll）：所有部门 spent/remaining 可见
	ovAll, err := st.LeftoverOverview(ctx, nil, true)
	if err != nil {
		t.Fatalf("overview all: %v", err)
	}
	if ovAll.My != nil {
		t.Fatalf("no dept, my must be nil: %+v", ovAll.My)
	}
	for _, d := range ovAll.Departments {
		switch d.ID {
		case deptA:
			if d.Spent == nil || *d.Spent != 0 {
				t.Fatalf("deptA spent=%v", d.Spent)
			}
		case deptB:
			// B 落标出价 300（c1）仍占用预算；c2 的 150 已随录取释放
			if d.Spent == nil || *d.Spent != 300 {
				t.Fatalf("deptB spent=%v", d.Spent)
			}
		}
	}
}

// TestLeftoverSettlementPhase 进入结算阶段自动按出价结算全部竞拍（幂等）：
// 赢家 = 最高出价部门、同额取先出价者；结算阶段竞拍数据只读（禁止出价）。
func TestLeftoverSettlementPhase(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	deptA, err := st.CreateDepartment(ctx, "部门A", "", 20)
	if err != nil {
		t.Fatalf("create deptA: %v", err)
	}
	deptB, err := st.CreateDepartment(ctx, "部门B", "", 20)
	if err != nil {
		t.Fatalf("create deptB: %v", err)
	}
	c1 := mustCreateCandidate(ctx, st, "甲", "")
	c2 := mustCreateCandidate(ctx, st, "乙", "")

	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set leftover: %v", err)
	}
	// c1：A 先出 500、B 后出 500（同额 → 先出价的 A 胜）；c2：B 出 300
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptA, 500); err != nil {
		t.Fatalf("bid A c1: %v", err)
	}
	if _, err := st.UpsertLeftoverBid(ctx, c1, deptB, 500); err != nil {
		t.Fatalf("bid B c1: %v", err)
	}
	if _, err := st.UpsertLeftoverBid(ctx, c2, deptB, 300); err != nil {
		t.Fatalf("bid B c2: %v", err)
	}

	// 结算前：结果未落库，仅只读计算。
	// 部门 A 视角：本部门出价的 c1 可见（未成交）；他部门出价的 c2 保密不可见。
	finals, err := st.LeftoverFinalResults(ctx, &deptA, false)
	if err != nil {
		t.Fatalf("final results: %v", err)
	}
	if len(finals) != 1 || finals[0].CandidateID != c1 ||
		finals[0].DepartmentID != deptA || finals[0].Amount != 500 || finals[0].Resolved {
		t.Fatalf("deptA finals=%+v", finals)
	}
	// 管理端（exposeAll）：两个候选人的计算结果全可见
	finalsAll, err := st.LeftoverFinalResults(ctx, nil, true)
	if err != nil {
		t.Fatalf("final results all: %v", err)
	}
	byCand := map[uint64]dsmodel.LeftoverFinalResult{}
	for _, f := range finalsAll {
		byCand[f.CandidateID] = *f
	}
	if len(finalsAll) != 2 || byCand[c1].DepartmentID != deptA || byCand[c1].Amount != 500 || byCand[c1].Resolved {
		t.Fatalf("c1 final=%+v", byCand[c1])
	}
	if byCand[c2].DepartmentID != deptB || byCand[c2].Amount != 300 {
		t.Fatalf("c2 final=%+v", byCand[c2])
	}

	// 进入结算阶段：自动结算全部竞拍 —— c1 同额取先出价的 A、c2 唯一出价者 B
	ch, cancel := st.Subscribe(0, 0)
	defer cancel()
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseSettlement); err != nil {
		t.Fatalf("set settlement: %v", err)
	}
	resolved := drainResolved(ch)
	if ref := resolved[c1]; ref.DepartmentID != deptA || ref.Amount != 500 {
		t.Fatalf("c1 resolved=%+v", ref)
	}
	if ref := resolved[c2]; ref.DepartmentID != deptB || ref.Amount != 300 {
		t.Fatalf("c2 resolved=%+v", ref)
	}

	// 结算阶段竞拍只读：禁止出价
	if _, err := st.UpsertLeftoverBid(ctx, c2, deptA, 100); err == nil {
		t.Fatalf("expected not_leftover_phase on bid")
	}

	// 结果落库：c1 A 500、c2 B 300
	results, err := st.ListLeftoverResults(ctx)
	if err != nil {
		t.Fatalf("results: %v", err)
	}
	got := map[uint64]dsmodel.LeftoverResult{}
	for _, r := range results {
		got[r.CandidateID] = *r
	}
	if got[c1].DepartmentID != deptA || got[c1].Amount != 500 {
		t.Fatalf("c1 result=%+v", got[c1])
	}
	if got[c2].DepartmentID != deptB || got[c2].Amount != 300 {
		t.Fatalf("c2 result=%+v", got[c2])
	}

	// 幂等：回捡漏阶段再进结算阶段 → 不重复成交、结果不变
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("back to leftover: %v", err)
	}
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseSettlement); err != nil {
		t.Fatalf("set settlement again: %v", err)
	}
	if again := drainResolved(ch); len(again) != 0 {
		t.Fatalf("re-settlement must be a no-op: %+v", again)
	}
	results, err = st.ListLeftoverResults(ctx)
	if err != nil || len(results) != 2 {
		t.Fatalf("results after re-settlement=%+v err=%v", results, err)
	}

	// 最终投影：两人均已成交（Resolved），成交部门/金额与落库一致（已成交结果对全员公开）
	finals, err = st.LeftoverFinalResults(ctx, &deptA, false)
	if err != nil {
		t.Fatalf("final results after settle: %v", err)
	}
	byCand = map[uint64]dsmodel.LeftoverFinalResult{}
	for _, f := range finals {
		byCand[f.CandidateID] = *f
	}
	if !byCand[c1].Resolved || byCand[c1].DepartmentID != deptA || byCand[c1].Amount != 500 {
		t.Fatalf("c1 final=%+v", byCand[c1])
	}
	if !byCand[c2].Resolved || byCand[c2].DepartmentID != deptB || byCand[c2].Amount != 300 {
		t.Fatalf("c2 final=%+v", byCand[c2])
	}
}

// TestAdmissionStatusStages 面试完成后的录取档状态机：
// COMPLETED → ADMISSION_PENDING → ADMITTED，重置不需要房间绑定；可回退到未签到。
func TestAdmissionStatusStages(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	c := mustCreateCandidate(ctx, st, "录取档", "")

	// 直接重置到 COMPLETED 需要房间（既有规则），此处走 签到→拉房→完成
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set phase: %v", err)
	}
	if _, err := st.CheckIn(ctx, c); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	ev, err := st.CreateRoom(ctx, "")
	if err != nil {
		t.Fatalf("create room: %v", err)
	}
	ref, ok := ev.Data.(state.RoomRef)
	if !ok {
		t.Fatalf("room event payload=%+v", ev.Data)
	}
	if _, err := st.PullCandidate(ctx, ref.RoomID, c); err != nil {
		t.Fatalf("pull: %v", err)
	}
	if _, err := st.ResetCandidateStatus(ctx, c, dsmodel.StatusCompleted); err != nil {
		t.Fatalf("complete: %v", err)
	}

	// 录取档推进：不需要房间
	if _, err := st.ResetCandidateStatus(ctx, c, dsmodel.StatusAdmissionPending); err != nil {
		t.Fatalf("to admission pending: %v", err)
	}
	if _, err := st.ResetCandidateStatus(ctx, c, dsmodel.StatusAdmitted); err != nil {
		t.Fatalf("to admitted: %v", err)
	}
	got, err := st.GetCandidate(ctx, c)
	if err != nil || got.Status != dsmodel.StatusAdmitted {
		t.Fatalf("status=%+v err=%v", got, err)
	}

	// 非法跳转：ADMITTED → IN_PROGRESS 不在转移表中
	if _, err := st.ResetCandidateStatus(ctx, c, dsmodel.StatusInProgress); err == nil {
		t.Fatalf("expected illegal transition")
	}

	// 回退到未签到（任意回退允许）
	if _, err := st.ResetCandidateStatus(ctx, c, dsmodel.StatusNotCheckedIn); err != nil {
		t.Fatalf("backward reset: %v", err)
	}
}

// TestLeftoverContestedAdmissions 多部门同时录取的候选人属争议：
// 不算已结算、可进入捡漏竞拍；结算后最高出价部门录取、其余部门（含未出价的手动录取）withdrawn。
func TestLeftoverContestedAdmissions(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	deptA, err := st.CreateDepartment(ctx, "部门A", "", 20)
	if err != nil {
		t.Fatalf("create deptA: %v", err)
	}
	deptB, err := st.CreateDepartment(ctx, "部门B", "", 20)
	if err != nil {
		t.Fatalf("create deptB: %v", err)
	}
	c := mustCreateCandidate(ctx, st, "争议者", "")

	// 录取阶段：A、B 两家都手动录取 → 争议
	if err := st.UpsertCandidateAdmission(ctx, c, deptA, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit A: %v", err)
	}
	if err := st.UpsertCandidateAdmission(ctx, c, deptB, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit B: %v", err)
	}

	// 争议候选人不算已结算：结果列表为空、未被封盘（A 可出价）
	results, err := st.ListLeftoverResults(ctx)
	if err != nil {
		t.Fatalf("results: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("contested candidate must not be settled: %+v", results)
	}
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set leftover: %v", err)
	}
	if _, err := st.UpsertLeftoverBid(ctx, c, deptA, 200); err != nil {
		t.Fatalf("bid on contested candidate: %v", err)
	}

	// 进入结算阶段：自动按出价仲裁 → A 唯一最高价录取；B 的手动录取改 withdrawn
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseSettlement); err != nil {
		t.Fatalf("set settlement: %v", err)
	}
	all, err := st.ListCandidateAdmissions(ctx, nil)
	if err != nil {
		t.Fatalf("list admissions: %v", err)
	}
	statusByDept := map[uint64]dsmodel.AdmissionStatus{}
	for _, a := range all {
		if a.CandidateID == c {
			statusByDept[a.DepartmentID] = a.Status
		}
	}
	if statusByDept[deptA] != dsmodel.AdmissionAdmitted || statusByDept[deptB] != dsmodel.AdmissionWithdrawn {
		t.Fatalf("admissions=%+v", statusByDept)
	}

	// 结算后：结果落库（成交额 = A 出价 200），且封盘（唯一录取）
	results, err = st.ListLeftoverResults(ctx)
	if err != nil || len(results) != 1 || results[0].CandidateID != c ||
		results[0].DepartmentID != deptA || results[0].Amount != 200 {
		t.Fatalf("results=%+v err=%v", results, err)
	}
	if _, err := st.UpsertLeftoverBid(ctx, c, deptB, 100); err == nil {
		t.Fatalf("expected not_leftover_phase after arbitration")
	}
}

// TestPhaseSwitchSyncsAdmissionStatuses 切换到捡漏/结算阶段时批量同步录取档：
// 唯一录取（已结算）→ 已录取；其余（面试已结束/争议/无决定）→ 待录取；未完成候选人不动。
func TestPhaseSwitchSyncsAdmissionStatuses(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	deptA, err := st.CreateDepartment(ctx, "部门A", "", 20)
	if err != nil {
		t.Fatalf("create deptA: %v", err)
	}

	settled := mustCreateCandidate(ctx, st, "唯一录取", "")   // 一家 admitted → 已录取
	contested := mustCreateCandidate(ctx, st, "争议", "")   // 两家 admitted → 待录取
	none := mustCreateCandidate(ctx, st, "无决定", "")       // 无决定 → 待录取
	inProgress := mustCreateCandidate(ctx, st, "面试中", "") // 未完成 → 不动

	for _, c := range []uint64{settled, contested, none} {
		if _, err := st.CheckIn(ctx, c); err != nil {
			t.Fatalf("checkin %d: %v", c, err)
		}
		roomEv, err := st.CreateRoom(ctx, "")
		if err != nil {
			t.Fatalf("create room: %v", err)
		}
		ref := roomEv.Data.(state.RoomRef)
		if _, err := st.PullCandidate(ctx, ref.RoomID, c); err != nil {
			t.Fatalf("pull %d: %v", c, err)
		}
		if _, err := st.ResetCandidateStatus(ctx, c, dsmodel.StatusCompleted); err != nil {
			t.Fatalf("complete %d: %v", c, err)
		}
	}
	if _, err := st.CheckIn(ctx, inProgress); err != nil {
		t.Fatalf("checkin inProgress: %v", err)
	}

	// 录取决定：settled 只 A admitted；contested A、B 都 admitted
	if err := st.UpsertCandidateAdmission(ctx, settled, deptA, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit settled: %v", err)
	}
	if err := st.UpsertCandidateAdmission(ctx, contested, deptA, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit contested A: %v", err)
	}
	deptB, err := st.CreateDepartment(ctx, "部门B", "", 20)
	if err != nil {
		t.Fatalf("create deptB: %v", err)
	}
	if err := st.UpsertCandidateAdmission(ctx, contested, deptB, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit contested B: %v", err)
	}

	// 切换到捡漏阶段 → 同步录取档
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set leftover: %v", err)
	}
	statusOf := map[uint64]dsmodel.CandidateStatus{}
	for _, c := range []uint64{settled, contested, none, inProgress} {
		got, err := st.GetCandidate(ctx, c)
		if err != nil {
			t.Fatalf("get %d: %v", c, err)
		}
		statusOf[c] = got.Status
	}
	if statusOf[settled] != dsmodel.StatusAdmitted {
		t.Fatalf("settled=%s, want ADMITTED", statusOf[settled])
	}
	if statusOf[contested] != dsmodel.StatusAdmissionPending {
		t.Fatalf("contested=%s, want ADMISSION_PENDING", statusOf[contested])
	}
	if statusOf[none] != dsmodel.StatusAdmissionPending {
		t.Fatalf("none=%s, want ADMISSION_PENDING", statusOf[none])
	}
	if statusOf[inProgress] != dsmodel.StatusCheckedInPendingAssign {
		t.Fatalf("inProgress=%s, want untouched", statusOf[inProgress])
	}

	// 争议候选人在捡漏阶段获最高出价：进入结算阶段自动成交 → 推进到已录取
	if _, err := st.UpsertLeftoverBid(ctx, contested, deptA, 100); err != nil {
		t.Fatalf("bid: %v", err)
	}
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseSettlement); err != nil {
		t.Fatalf("set settlement: %v", err)
	}
	got, err := st.GetCandidate(ctx, contested)
	if err != nil || got.Status != dsmodel.StatusAdmitted {
		t.Fatalf("contested after settle=%+v err=%v", got, err)
	}
}

// 面试计时打点：进入「面试中」写入 interview_started_at，离开即清空，重新进入重新打点。
// 该字段独立于 updated_at —— 资料编辑会刷新 updated_at，计时起点不能依赖它。
func TestInterviewStartedAt(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	id := mustCreateCandidate(ctx, st, "计时", "简介")

	startedAt := func() *time.Time {
		t.Helper()
		c, err := st.GetCandidate(ctx, id)
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		return c.InterviewStartedAt
	}
	mustNil := func(stage string) {
		t.Helper()
		if got := startedAt(); got != nil {
			t.Fatalf("%s：interview_started_at 应为空，实得 %v", stage, got)
		}
	}

	mustNil("创建后")
	if _, err := st.CheckIn(ctx, id); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, id); err != nil {
		t.Fatalf("pull: %v", err)
	}
	mustNil("待面试")

	// 推进阶段需房间成员身份（无主持人概念）
	if _, _, err := st.JoinRoom(ctx, roomID, 1); err != nil {
		t.Fatalf("join: %v", err)
	}

	// 进入面试中 → 打点
	lo := time.Now().Add(-time.Second)
	if _, err := st.MovePhase(ctx, roomID, 1, dsmodel.StatusInProgress); err != nil {
		t.Fatalf("move in_progress: %v", err)
	}
	first := startedAt()
	if first == nil {
		t.Fatal("进入面试中应打点 interview_started_at")
	}
	if first.Before(lo) || first.After(time.Now().Add(time.Second)) {
		t.Fatalf("打点时刻应落在本次迁移附近：%v", first)
	}

	// 资料编辑刷新 updated_at，但不得改动计时起点
	c, err := st.GetCandidate(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if _, err := st.UpdateCandidate(ctx, id, info3(c.StudentNo, "计时改", "简介改")); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got := startedAt(); got == nil || !got.Equal(*first) {
		t.Fatalf("资料编辑不应改动计时起点：%v → %v", first, got)
	}

	// 离开面试中 → 清空；重新进入 → 重新打点
	time.Sleep(2 * time.Millisecond)
	if _, err := st.ResetCandidateStatus(ctx, id, dsmodel.StatusAssigned); err != nil {
		t.Fatalf("reset assigned: %v", err)
	}
	mustNil("重置回待面试")
	if _, err := st.ResetCandidateStatus(ctx, id, dsmodel.StatusInProgress); err != nil {
		t.Fatalf("reset in_progress: %v", err)
	}
	second := startedAt()
	if second == nil || !second.After(*first) {
		t.Fatalf("重新进入面试中应重新打点：%v → %v", first, second)
	}

	// 完成 → 清空
	if _, err := st.ResetCandidateStatus(ctx, id, dsmodel.StatusCompleted); err != nil {
		t.Fatalf("reset completed: %v", err)
	}
	mustNil("面试已结束")
}

// 出价允许 0：0 是合法出价（落库可见）；重复提交 0 走更新而不是重复插入（唯一索引不炸）；
// 0 ↔ 正数 之间来回改价都只有一条记录；负数仍被拒绝。
func TestZeroBidAllowed(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	dept, err := st.CreateDepartment(ctx, "零价组", "", 10)
	if err != nil {
		t.Fatalf("create dept: %v", err)
	}
	cand := mustCreateCandidate(ctx, st, "零价候选人", "")
	mustCompleteCandidate(ctx, st, cand)
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set phase: %v", err)
	}

	if _, err := st.UpsertLeftoverBid(ctx, cand, dept, -1); err == nil {
		t.Fatal("负数出价应被拒绝")
	}

	bidOf := func() int {
		t.Helper()
		bids, err := st.ListLeftoverBids(ctx, &dept)
		if err != nil {
			t.Fatalf("list bids: %v", err)
		}
		if len(bids) != 1 {
			t.Fatalf("应恰好一条出价，实得 %d 条：%+v", len(bids), bids)
		}
		return bids[0].Amount
	}

	if _, err := st.UpsertLeftoverBid(ctx, cand, dept, 0); err != nil {
		t.Fatalf("0 出价应被接受: %v", err)
	}
	if got := bidOf(); got != 0 {
		t.Fatalf("出价应为 0，实得 %d", got)
	}
	// 重复提交 0：必须走更新分支（历史缺陷：oldAmount > 0 当存在性哨兵 → 重复插入撞唯一索引）
	if _, err := st.UpsertLeftoverBid(ctx, cand, dept, 0); err != nil {
		t.Fatalf("重复 0 出价应走更新: %v", err)
	}
	if got := bidOf(); got != 0 {
		t.Fatalf("重复 0 后仍应为 0，实得 %d", got)
	}
	if _, err := st.UpsertLeftoverBid(ctx, cand, dept, 150); err != nil {
		t.Fatalf("0 改价为正数: %v", err)
	}
	if got := bidOf(); got != 150 {
		t.Fatalf("改价后应为 150，实得 %d", got)
	}
	if _, err := st.UpsertLeftoverBid(ctx, cand, dept, 0); err != nil {
		t.Fatalf("改回 0: %v", err)
	}
	if got := bidOf(); got != 0 {
		t.Fatalf("改回 0 后应为 0，实得 %d", got)
	}
}

// 争议候选人的预算口径：不算已录取（不缩预算基数）、其出价照常占用预算；
// 只有唯一录取（封盘）的赢家候选人才计入已录取并释放自己的出价占用。
func TestLeftoverBudgetForContestedAdmissions(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	deptA, err := st.CreateDepartment(ctx, "部门A", "", 10)
	if err != nil {
		t.Fatalf("create deptA: %v", err)
	}
	deptB, err := st.CreateDepartment(ctx, "部门B", "", 10)
	if err != nil {
		t.Fatalf("create deptB: %v", err)
	}
	contested := mustCreateCandidate(ctx, st, "争议者", "")
	solo := mustCreateCandidate(ctx, st, "唯一录取者", "")

	// A、B 两家同时手动录取 contested → 争议
	if err := st.UpsertCandidateAdmission(ctx, contested, deptA, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit A: %v", err)
	}
	if err := st.UpsertCandidateAdmission(ctx, contested, deptB, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit B: %v", err)
	}
	if err := st.SetSystemStatus(ctx, dsmodel.SystemPhaseLeftover); err != nil {
		t.Fatalf("set leftover: %v", err)
	}
	// A 对争议候选人出价 100（争议未封盘 → 允许出价）
	if _, err := st.UpsertLeftoverBid(ctx, contested, deptA, 100); err != nil {
		t.Fatalf("bid A contested: %v", err)
	}
	// B 先对 solo 出价 150（此时尚未录取 → 允许出价），随后 B 唯一录取 solo（封盘）→ 该出价应随录取释放
	if _, err := st.UpsertLeftoverBid(ctx, solo, deptB, 150); err != nil {
		t.Fatalf("bid B solo: %v", err)
	}
	if err := st.UpsertCandidateAdmission(ctx, solo, deptB, dsmodel.AdmissionAdmitted); err != nil {
		t.Fatalf("admit B solo: %v", err)
	}

	ov, err := st.LeftoverOverview(ctx, nil, true)
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	byID := map[uint64]dsmodel.DepartmentLeftover{}
	for _, d := range ov.Departments {
		byID[d.ID] = d
	}
	a := byID[deptA]
	if a.AdmittedCount != 0 {
		t.Fatalf("争议候选人不应计入已录取：%+v", a)
	}
	if a.Budget != 1000 {
		t.Fatalf("争议候选人不应缩预算基数（期望 10*100=1000）：%+v", a)
	}
	if a.Spent == nil || *a.Spent != 100 {
		t.Fatalf("争议候选人的出价应占用预算：%+v", a)
	}
	b := byID[deptB]
	if b.AdmittedCount != 1 {
		t.Fatalf("唯一录取应计入已录取：%+v", b)
	}
	if b.Budget != 900 {
		t.Fatalf("唯一录取应缩预算基数（期望 (10-1)*100=900）：%+v", b)
	}
	if b.Spent == nil || *b.Spent != 0 {
		t.Fatalf("唯一录取赢家的出价应释放：%+v", b)
	}
}
