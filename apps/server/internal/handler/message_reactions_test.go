package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// TestMessageReactionEventCarriesActor 表情回复事件必须带回复人的展示名与部门（WS 线上载荷）：
// 前端「谁回了什么」的明细在实时路径上只能靠事件——面试官没有列用户的权限、查不到用户表，
// 事件不带名字就只剩「面试官 {id}」，右键菜单里也就看不到真实姓名。
func TestMessageReactionEventCarriesActor(t *testing.T) {
	r := newTestApp(t)
	admin := adminToken(t, r)

	// 一位有部门、持 rooms.chat 的面试官
	_, out := doJSON(t, r, "POST", "/api/departments", `{"name":"研发部","description":""}`, admin)
	deptID := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	adminID := int(out["user"].(map[string]any)["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"chat-only","description":"","permissions":["rooms.view","rooms.chat"]}`, admin)
	roleID := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/users",
		`{"username":"reactor","name":"张三","password":"pass","role_ids":[`+itoa(roleID)+`],"department_id":`+itoa(deptID)+`}`, admin)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"reactor","password":"pass"}`, "")
	reactor := out["token"].(string)

	// 房间 + 候选人 + 一条记录
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024500","name":"被回复的人"}`, admin)
	candID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/candidates/"+itoa(candID)+"/check-in", "", admin)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", admin)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/rooms/"+itoa(roomID)+"/candidate", `{"candidate_id":`+itoa(candID)+`}`, admin)
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/members", `{"user_id":`+itoa(adminID)+`}`, admin)
	code, out := doJSON(t, r, "POST", "/api/candidates/"+itoa(candID)+"/messages", `{"content":"待评价"}`, admin)
	if code != http.StatusCreated {
		t.Fatalf("append: %d %v", code, out)
	}
	msgID := int(out["id"].(float64))

	// admin 在房间里挂一条 WS 连接（观察别人的表情事件）
	srv := httptest.NewServer(r)
	defer srv.Close()
	conn, _, err := websocket.DefaultDialer.Dial(
		"ws"+strings.TrimPrefix(srv.URL, "http")+"/ws/rooms/"+itoa(roomID), nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(map[string]any{"op": "auth", "req_id": "a1", "data": map[string]string{"token": admin}}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	authed := false
	for i := 0; i < 10 && !authed; i++ {
		var env map[string]any
		if err := conn.ReadJSON(&env); err != nil {
			t.Fatalf("read auth env: %v", err)
		}
		if env["type"] == "reply" && env["req_id"] == "a1" {
			authed = true
		}
	}
	if !authed {
		t.Fatal("auth reply not received")
	}

	// 面试官回一个表情：现场抓到的事件必须自带「张三 · 研发部」
	if code, _ := doJSON(t, r, "PUT",
		"/api/candidates/"+itoa(candID)+"/messages/"+itoa(msgID)+"/reactions/👍", "", reactor); code != http.StatusOK {
		t.Fatalf("react: %d", code)
	}
	for i := 0; i < 20; i++ {
		var env map[string]any
		if err := conn.ReadJSON(&env); err != nil {
			t.Fatalf("read env: %v", err)
		}
		if env["type"] != "message_reactions_changed" {
			continue
		}
		envelopeData, _ := env["data"].(map[string]any)
		inner, _ := envelopeData["data"].(map[string]any)
		if inner["UserName"] != "张三" || inner["UserDepartment"] != "研发部" {
			t.Fatalf("表情事件应带回复人展示名与部门: %v", inner)
		}
		if inner["Emoji"] != "👍" || inner["Added"] != true {
			t.Fatalf("表情事件载荷不符: %v", inner)
		}
		return
	}
	t.Fatal("未收到表情回复事件")
}
