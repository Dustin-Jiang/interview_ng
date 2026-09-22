package handler_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
)

// adminToken 登录种子管理员并返回 token（测试辅助）。
func adminToken(t *testing.T, r *gin.Engine) string {
	t.Helper()
	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("admin login failed: %v", out)
	}
	return token
}

// rowErrors 从 400 响应体提取行级错误报告（index → 原因）。
func rowErrors(t *testing.T, out map[string]any) map[int]string {
	t.Helper()
	raw, ok := out["rows"].([]any)
	if !ok {
		t.Fatalf("响应缺少行级报告 rows: %v", out)
	}
	got := map[int]string{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("行级报告元素形状不符: %v", item)
		}
		got[int(m["index"].(float64))] = m["error"].(string)
	}
	return got
}

// TestCreateCandidateStudentNo 学号必填：缺学号 → 400；全角数字按半角归一化落库。
func TestCreateCandidateStudentNo(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	if code, out := doJSON(t, r, "POST", "/api/candidates", `{"name":"张三"}`, token); code != http.StatusBadRequest {
		t.Fatalf("缺学号应 400: got %d %v", code, out)
	}
	if code, out := doJSON(t, r, "POST", "/api/candidates", `{"student_no":"12a","name":"张三"}`, token); code != http.StatusBadRequest {
		t.Fatalf("非数字学号应 400: got %d %v", code, out)
	}

	// 全角数字 → 半角，前导零保留
	_, out := doJSON(t, r, "POST", "/api/candidates", `{"student_no":" ００１２３ ","name":"张三","profile":"后端"}`, token)
	id := int(out["id"].(float64))
	if id == 0 {
		t.Fatalf("create: %v", out)
	}
	code, got := doJSON(t, r, "GET", "/api/candidates/"+itoa(id), "", token)
	if code != http.StatusOK || got["student_no"] != "00123" {
		t.Fatalf("归一化落库: got %d %v", code, got)
	}
}

// TestCandidateStudentNoConflict 学号唯一：新增/编辑撞号 → 409；编辑保留自己的学号 → 200。
func TestCandidateStudentNoConflict(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	_, out := doJSON(t, r, "POST", "/api/candidates", `{"student_no":"0001","name":"甲"}`, token)
	a := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"0002","name":"乙"}`, token)
	b := int(out["id"].(float64))

	if code, out := doJSON(t, r, "POST", "/api/candidates", `{"student_no":"0001","name":"丙"}`, token); code != http.StatusConflict {
		t.Fatalf("新增撞号应 409: got %d %v", code, out)
	}
	if code, out := doJSON(t, r, "PUT", "/api/candidates/"+itoa(b), `{"student_no":"0001","name":"乙"}`, token); code != http.StatusConflict {
		t.Fatalf("编辑撞号应 409: got %d %v", code, out)
	}
	if code, out := doJSON(t, r, "PUT", "/api/candidates/"+itoa(a), `{"student_no":"0001","name":"甲改","profile":"p"}`, token); code != http.StatusOK {
		t.Fatalf("编辑保留自己的学号应 200: got %d %v", code, out)
	}
	// 编辑可改到未占用学号
	if code, _ := doJSON(t, r, "PUT", "/api/candidates/"+itoa(b), `{"student_no":"0003","name":"乙"}`, token); code != http.StatusOK {
		t.Fatalf("改到空闲学号应 200: got %d", code)
	}
	code, got := doJSON(t, r, "GET", "/api/candidates/"+itoa(b), "", token)
	if code != http.StatusOK || got["student_no"] != "0003" {
		t.Fatalf("改学号未生效: %d %v", code, got)
	}
}

// TestCandidateInfoFieldsJSON 资料字段（志愿/调剂/联系方式）走扁平 JSON 契约：
// 创建即落库并由 GET 回显；编辑全量覆盖（零值清空）。
func TestCandidateInfoFieldsJSON(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	body := `{"student_no":"0070","name":"张三","first_choice":"智能科学与技术","second_choice":"软件工程",` +
		`"accept_adjust":true,"phone":"13800000000","qq":"10001","email":"zs@example.com"}`
	_, out := doJSON(t, r, "POST", "/api/candidates", body, token)
	id := int(out["id"].(float64))
	_, got := doJSON(t, r, "GET", "/api/candidates/"+itoa(id), "", token)
	if got["first_choice"] != "智能科学与技术" || got["second_choice"] != "软件工程" ||
		got["accept_adjust"] != true || got["phone"] != "13800000000" ||
		got["qq"] != "10001" || got["email"] != "zs@example.com" {
		t.Fatalf("创建未回显新字段: %v", got)
	}

	if code, _ := doJSON(t, r, "PUT", "/api/candidates/"+itoa(id), `{"student_no":"0070","name":"张三"}`, token); code != http.StatusOK {
		t.Fatalf("编辑应 200: %d", code)
	}
	_, got = doJSON(t, r, "GET", "/api/candidates/"+itoa(id), "", token)
	for _, k := range []string{"first_choice", "second_choice", "phone", "qq", "email"} {
		if got[k] != "" {
			t.Fatalf("编辑应清空可选字段 %s: %v", k, got)
		}
	}
	if got["accept_adjust"] != false {
		t.Fatalf("accept_adjust 零值应覆盖为 false: %v", got)
	}
}

// TestImportCandidatesEndpoint 导入接口端到端：整批成功 → 行级报告；
// 再次导入同学号覆盖资料（运行态不变）；学号可被关键词检索命中。
func TestImportCandidatesEndpoint(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	body := `{"rows":[{"student_no":"0101","name":"甲","profile":"后端"},{"student_no":"0102","name":"乙","profile":""}]}`
	code, out := doJSON(t, r, "POST", "/api/candidates/imports", body, token)
	if code != http.StatusOK {
		t.Fatalf("import: got %d %v", code, out)
	}
	if out["created"].(float64) != 2 || out["updated"].(float64) != 0 {
		t.Fatalf("报告计数: %v", out)
	}
	rows, _ := out["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("行级报告: %v", out)
	}
	first := rows[0].(map[string]any)
	if int(first["index"].(float64)) != 0 || first["status"] != "created" {
		t.Fatalf("行级报告形状: %v", first)
	}
	candID := int(first["candidate_id"].(float64))

	// 学号可被检索命中
	code, got := doJSON(t, r, "GET", "/api/candidates?q=0101", "", token)
	items, _ := got["items"].([]any)
	if code != http.StatusOK || len(items) != 1 {
		t.Fatalf("按学号检索: got %d %v", code, got)
	}
	if items[0].(map[string]any)["student_no"] != "0101" {
		t.Fatalf("检索命中项: %v", items[0])
	}

	// 再次导入：0101 覆盖资料 + 0102 无变化 + 新增 0103
	body = `{"rows":[{"student_no":"0101","name":"甲改","profile":"新简介"},{"student_no":"0102","name":"乙","profile":""},{"student_no":"0103","name":"丙","profile":""}]}`
	code, out = doJSON(t, r, "POST", "/api/candidates/imports", body, token)
	if code != http.StatusOK || out["created"].(float64) != 1 || out["updated"].(float64) != 2 {
		t.Fatalf("二次导入: got %d %v", code, out)
	}
	code, got = doJSON(t, r, "GET", "/api/candidates/"+itoa(candID), "", token)
	if code != http.StatusOK || got["name"] != "甲改" || got["profile"] != "新简介" || got["student_no"] != "0101" {
		t.Fatalf("覆盖结果: %d %v", code, got)
	}
}

// TestImportCandidatesRowErrorsAtomic 全或无：任一行不合法 → 400 + 全量行级错误，整批不落库。
func TestImportCandidatesRowErrorsAtomic(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	body := `{"rows":[{"student_no":"0201","name":"合法"},{"student_no":"","name":"缺学号"},{"student_no":"0202","name":""},{"student_no":"x2","name":"学号非法"}]}`
	code, out := doJSON(t, r, "POST", "/api/candidates/imports", body, token)
	if code != http.StatusBadRequest {
		t.Fatalf("应整批拒绝: got %d %v", code, out)
	}
	errs := rowErrors(t, out)
	if len(errs) != 3 || errs[1] == "" || errs[2] == "" || errs[3] == "" {
		t.Fatalf("行级错误报告: %v", errs)
	}
	code, got := doJSON(t, r, "GET", "/api/candidates", "", token)
	items, _ := got["items"].([]any)
	if code != http.StatusOK || len(items) != 0 {
		t.Fatalf("整批应回滚: got %d %v", code, got)
	}
	// 空批次 → 400
	if code, out := doJSON(t, r, "POST", "/api/candidates/imports", `{"rows":[]}`, token); code != http.StatusBadRequest {
		t.Fatalf("空批次应 400: got %d %v", code, out)
	}
}

// TestImportCandidatesForbidden 导入需 candidates.manage：仅 rooms.view 的用户 → 403。
func TestImportCandidatesForbidden(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	_, out := doJSON(t, r, "POST", "/api/roles", `{"name":"reader","description":"","permissions":["rooms.view","candidates.create"]}`, token)
	roleID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/users", `{"username":"imp-reader","name":"","password":"pass","role_ids":[`+strconv.Itoa(roleID)+`]}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"imp-reader","password":"pass"}`, "")
	rToken := out["token"].(string)

	if code, out := doJSON(t, r, "POST", "/api/candidates/imports", `{"rows":[{"student_no":"0301","name":"甲"}]}`, rToken); code != http.StatusForbidden {
		t.Fatalf("无 candidates.manage 应 403: got %d %v", code, out)
	}
}
