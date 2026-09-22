package state_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/state"
)

// TestCreateCandidateStudentNoValidation 学号是身份键：必填、纯数字、≤64 位；
// 归一化去除首尾空白并把全角数字折为半角，前导零原样保留。
func TestCreateCandidateStudentNoValidation(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// 非法学号：空、纯空白、含非数字、含内部空白、超长
	for _, raw := range []string{"", "   ", "12a3", "12 34", "1.5", "-1", strings.Repeat("9", 65)} {
		if _, err := st.CreateCandidate(ctx, info3(raw, "张三", "")); err == nil {
			t.Fatalf("student_no %q: 期望拒绝，实际通过", raw)
		}
	}
	// 非法输入不得落库
	if got, err := st.ListCandidates(ctx, "", "", 50, 0); err != nil || len(got) != 0 {
		t.Fatalf("非法学号不应落库: n=%d err=%v", len(got), err)
	}

	// 归一化：全角数字 → 半角，首尾空白（含全角空格）去除，前导零保留
	for raw, want := range map[string]string{
		"  00123  ":        "00123",
		"１２３":              "123",
		"０００１":             "0001",
		"\u30000042\u3000": "0042",
	} {
		ev, err := st.CreateCandidate(ctx, info3(raw, "李四", ""))
		if err != nil {
			t.Fatalf("create %q: %v", raw, err)
		}
		c, err := st.GetCandidate(ctx, state.CandidateIDOf(ev))
		if err != nil {
			t.Fatalf("get: %v", err)
		}
		if c.StudentNo != want {
			t.Fatalf("归一化学号 %q: got %q want %q", raw, c.StudentNo, want)
		}
	}
}

// TestCandidateStudentNoUnique 学号唯一：新增撞号 → ErrStudentNoExists；编辑撞他人号同样拒绝，
// 编辑保留自己的学号不算冲突。
func TestCandidateStudentNoUnique(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	a := mustCreateCandidateWithNo(ctx, st, "0001", "甲", "")
	b := mustCreateCandidateWithNo(ctx, st, "0002", "乙", "")

	// 新增撞号（含等价归一化写法）
	for _, raw := range []string{"0001", " 0001 ", "０００１"} {
		_, err := st.CreateCandidate(ctx, info3(raw, "丙", ""))
		if !errors.Is(err, state.ErrStudentNoExists) {
			t.Fatalf("create %q: got %v, want ErrStudentNoExists", raw, err)
		}
	}

	// 编辑撞他人号 → 拒绝，且原值不变
	if _, err := st.UpdateCandidate(ctx, b, info3("0001", "乙", "")); !errors.Is(err, state.ErrStudentNoExists) {
		t.Fatalf("update to taken: got %v", err)
	}
	if c, _ := st.GetCandidate(ctx, b); c.StudentNo != "0002" {
		t.Fatalf("冲突被拒后学号不应变化: %s", c.StudentNo)
	}

	// 编辑保留自己的学号 → 允许
	if _, err := st.UpdateCandidate(ctx, a, info3("0001", "甲改", "简介")); err != nil {
		t.Fatalf("update self student_no: %v", err)
	}
	if c, _ := st.GetCandidate(ctx, a); c.StudentNo != "0001" || c.Name != "甲改" {
		t.Fatalf("update self: %+v", c)
	}

	// 编辑改到未占用的学号 → 允许
	if _, err := st.UpdateCandidate(ctx, a, info3("0009", "甲改", "")); err != nil {
		t.Fatalf("update free student_no: %v", err)
	}
	if c, _ := st.GetCandidate(ctx, a); c.StudentNo != "0009" {
		t.Fatalf("改学号: got %s", c.StudentNo)
	}
}

// TestImportCandidatesUpsert 导入按学号 upsert：未命中新建、命中覆盖姓名/简介（值相同也写），
// 学号本身不被覆盖，运行态（状态机 / 房间绑定）不受影响；报告与事件逐行给出。
func TestImportCandidatesUpsert(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	// 已有一位候选人，并且已签到 + 拉入房间（运行态：状态与房间绑定都必须存活于导入）
	existing := mustCreateCandidateWithNo(ctx, st, "0010", "旧名", "")
	if _, err := st.CheckIn(ctx, existing); err != nil {
		t.Fatalf("checkin: %v", err)
	}
	roomID := mustCreateRoom(ctx, st)
	if _, err := st.PullCandidate(ctx, roomID, existing); err != nil {
		t.Fatalf("pull: %v", err)
	}

	report, err := st.ImportCandidates(ctx, []state.CandidateImportRow{
		{CandidateInfo: info3("0010", "新名", "新简介")},
		{CandidateInfo: info3("0011", "新建", "")},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Created != 1 || report.Updated != 1 || len(report.Rows) != 2 {
		t.Fatalf("report: %+v", report)
	}
	if report.Rows[0] != (state.ImportOutcome{Index: 0, Status: state.ImportStatusUpdated, CandidateID: existing}) {
		t.Fatalf("row0: %+v", report.Rows[0])
	}
	if report.Rows[1].Index != 1 || report.Rows[1].Status != state.ImportStatusCreated {
		t.Fatalf("row1: %+v", report.Rows[1])
	}
	// 事件与行一一对应（service 层据此在落库后广播）
	if len(report.Events) != 2 || report.Events[0].Type != state.EventCandidateUpdated ||
		report.Events[1].Type != state.EventCandidateCreated {
		t.Fatalf("events: %+v", report.Events)
	}

	// 命中更新只改资料；状态机与房间绑定原封不动
	updated, err := st.GetCandidate(ctx, existing)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Name != "新名" || updated.Profile != "新简介" || updated.StudentNo != "0010" {
		t.Fatalf("updated: %+v", updated)
	}
	if updated.Status != dsmodel.StatusAssigned || updated.RoomID == nil || *updated.RoomID != roomID {
		t.Fatalf("运行态被导入改动: status=%s room=%v", updated.Status, updated.RoomID)
	}

	// 值相同也写：仍然报 updated（候选人的最新一次提交为准）
	again, err := st.ImportCandidates(ctx, []state.CandidateImportRow{
		{CandidateInfo: info3("0010", "新名", "新简介")},
	})
	if err != nil {
		t.Fatalf("import again: %v", err)
	}
	if again.Created != 0 || again.Updated != 1 {
		t.Fatalf("report again: %+v", again)
	}
}

// TestImportCandidatesBatchDuplicateLastWins 批内同学号：后行覆盖前行，
// 最终只留一条记录（先建后改），且行报告逐行如实给出。
func TestImportCandidatesBatchDuplicateLastWins(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	report, err := st.ImportCandidates(ctx, []state.CandidateImportRow{
		{CandidateInfo: info3("0020", "先", "p1")},
		{CandidateInfo: info3(" 0020 ", "后", "p2")},
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Created != 1 || report.Updated != 1 {
		t.Fatalf("report: %+v", report)
	}
	if report.Rows[0].Status != state.ImportStatusCreated || report.Rows[1].Status != state.ImportStatusUpdated {
		t.Fatalf("rows: %+v", report.Rows)
	}
	if report.Rows[0].CandidateID != report.Rows[1].CandidateID {
		t.Fatalf("批内重复应落在同一候选人: %+v", report.Rows)
	}

	all, err := st.ListCandidates(ctx, "", "0020", 50, 0)
	if err != nil || len(all) != 1 {
		t.Fatalf("重复学号应只留一条: n=%d err=%v", len(all), err)
	}
	if all[0].Name != "后" || all[0].Profile != "p2" {
		t.Fatalf("后行应覆盖前行: %+v", all[0])
	}
}

// TestImportCandidatesAtomicReject 全或无：任一行不合法即整批不落库，
// 报告一次列出全部问题行（行号 + 原因）。
func TestImportCandidatesAtomicReject(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	_, err := st.ImportCandidates(ctx, []state.CandidateImportRow{
		{CandidateInfo: info3("0030", "合法", "")},
		{CandidateInfo: info3("", "缺学号", "")},
		{CandidateInfo: info3("0031", "", "")},
		{CandidateInfo: info3("00A1", "学号非法", "")},
	})
	var ie *state.ImportError
	if !errors.As(err, &ie) {
		t.Fatalf("got %v, want *state.ImportError", err)
	}
	if len(ie.Rows) != 3 {
		t.Fatalf("应列出全部问题行: %+v", ie.Rows)
	}
	wantIdx := []int{1, 2, 3}
	for i, r := range ie.Rows {
		if r.Index != wantIdx[i] || r.Msg == "" {
			t.Fatalf("row error[%d]: %+v", i, r)
		}
	}
	// 合法行也一并回滚
	if got, err := st.ListCandidates(ctx, "", "", 50, 0); err != nil || len(got) != 0 {
		t.Fatalf("整批应回滚: n=%d err=%v", len(got), err)
	}

	// 空批次与超限批次由契约直接拒绝
	if _, err := st.ImportCandidates(ctx, nil); err == nil {
		t.Fatal("空批次应被拒绝")
	}
	rows := make([]state.CandidateImportRow, state.MaxImportRows+1)
	for i := range rows {
		rows[i] = state.CandidateImportRow{CandidateInfo: info3("0040", "同号", "")}
	}
	if _, err := st.ImportCandidates(ctx, rows); err == nil {
		t.Fatal("超限批次应被拒绝")
	}
	if got, _ := st.ListCandidates(ctx, "", "", 50, 0); len(got) != 0 {
		t.Fatalf("超限批次不应落库: n=%d", len(got))
	}
}

// TestListCandidatesSearchByStudentNo 关键词检索覆盖学号（子串匹配）。
func TestListCandidatesSearchByStudentNo(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	mustCreateCandidateWithNo(ctx, st, "0051", "张三", "后端")
	mustCreateCandidateWithNo(ctx, st, "0052", "李四", "前端")

	for _, tc := range []struct {
		q    string
		want int
	}{{"0051", 1}, {"005", 2}, {"张三", 1}, {"前端", 1}, {"00", 2}} {
		got, err := st.ListCandidates(ctx, "", tc.q, 50, 0)
		if err != nil {
			t.Fatalf("list %q: %v", tc.q, err)
		}
		if len(got) != tc.want {
			t.Fatalf("q=%q: got %d want %d", tc.q, len(got), tc.want)
		}
	}
}

// info3 构造仅含学号/姓名/简介的资料集（其余志愿/联系方式字段留空，测试辅助）。
func info3(studentNo, name, profile string) state.CandidateInfo {
	return state.CandidateInfo{StudentNo: studentNo, Name: name, Profile: profile}
}

// mustCreateCandidateWithNo 指定学号创建候选人并返回 id（测试辅助）。
func mustCreateCandidateWithNo(ctx context.Context, st state.StateStore, studentNo, name, profile string) uint64 {
	ev, err := st.CreateCandidate(ctx, info3(studentNo, name, profile))
	if err != nil {
		panic(err)
	}
	return state.CandidateIDOf(ev)
}

// TestCandidateInfoFields 资料字段（志愿/调剂/联系方式）随新建与编辑全量落库：
// 文本字段裁剪空白；编辑可清空可选字段（含 accept_adjust 的 false 零值覆盖）；
// 批量导入同样写入新列。
func TestCandidateInfoFields(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	full := state.CandidateInfo{
		StudentNo: "0060", Name: " 张三 ", Profile: " 后端 ",
		FirstChoice: " 智能科学与技术 ", SecondChoice: "软件工程",
		AcceptAdjust: true, Phone: " 13800000000 ", QQ: " 10001 ", Email: " zs@example.com ",
	}
	ev, err := st.CreateCandidate(ctx, full)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := state.CandidateIDOf(ev)
	c, err := st.GetCandidate(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if c.Name != "张三" || c.Profile != "后端" || c.FirstChoice != "智能科学与技术" ||
		c.SecondChoice != "软件工程" || !c.AcceptAdjust ||
		c.Phone != "13800000000" || c.QQ != "10001" || c.Email != "zs@example.com" {
		t.Fatalf("新建资料字段未全量落库/裁剪: %+v", c)
	}

	// 编辑清空全部可选字段（零值必须覆盖旧值）
	if _, err := st.UpdateCandidate(ctx, id, info3("0060", "张三", "")); err != nil {
		t.Fatalf("update: %v", err)
	}
	c, _ = st.GetCandidate(ctx, id)
	if c.FirstChoice != "" || c.SecondChoice != "" || c.AcceptAdjust ||
		c.Phone != "" || c.QQ != "" || c.Email != "" {
		t.Fatalf("编辑应全量覆盖（零值清空可选字段）: %+v", c)
	}

	// 导入同样写入新列
	rows := []state.CandidateImportRow{{CandidateInfo: state.CandidateInfo{
		StudentNo: "0061", Name: "李四",
		FirstChoice: "网络空间安全", AcceptAdjust: true, Phone: "13900000000",
	}}}
	if _, err := st.ImportCandidates(ctx, rows); err != nil {
		t.Fatalf("import: %v", err)
	}
	list, _ := st.ListCandidates(ctx, "", "0061", 10, 0)
	if len(list) != 1 || list[0].FirstChoice != "网络空间安全" || !list[0].AcceptAdjust ||
		list[0].Phone != "13900000000" || list[0].Email != "" {
		t.Fatalf("导入新列未落库: %+v", list)
	}
}
