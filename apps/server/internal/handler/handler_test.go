package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"interview_ng/internal/auth"
	"interview_ng/internal/broadcast"
	"interview_ng/internal/handler"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/oidcauth"
	"interview_ng/internal/rbac"
	"interview_ng/internal/seed"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

func newTestApp(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&dsmodel.User{}, &dsmodel.Candidate{}, &dsmodel.Room{},
		&dsmodel.RoomMember{}, &dsmodel.Message{},
		&dsmodel.Role{}, &dsmodel.RolePermission{}, &dsmodel.UserRole{},
		&dsmodel.Department{}, &dsmodel.SystemStatus{}, &dsmodel.CandidateAdmission{}, &dsmodel.Bid{},
		&dsmodel.OidcConfig{}, &dsmodel.OidcRoleRule{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cache := rbac.New()
	if err := seed.Init(context.Background(), db, cache); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store := state.NewMemStateStore(db)
	am := auth.New("test-secret", 7*24*time.Hour, db, cache, store)
	b := broadcast.New()
	svc := service.New(store, b)

	r := gin.New()
	handler.NewHTTPServer(svc, store, am, oidcauth.New()).RegisterRoutes(r)
	handler.NewWSServer(svc, b, store, am).RegisterRoutes(r)
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path, body, token string) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != "" {
		buf.WriteString(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	out := map[string]any{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

func TestLoginAndAuthFlow(t *testing.T) {
	r := newTestApp(t)

	// 未登录访问受保护接口 → 401
	if code, _ := doJSON(t, r, "GET", "/api/me", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("unauth me: got %d", code)
	}

	// 错误密码 → 401
	code, _ := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"wrong"}`, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("bad login: got %d", code)
	}

	// 正确登录 → 200 + token
	code, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	if code != http.StatusOK {
		t.Fatalf("login: got %d %v", code, out)
	}
	token, _ := out["token"].(string)
	if token == "" {
		t.Fatalf("no token: %v", out)
	}

	// /api/me 返回权限并集（admin 含全部）
	code, out = doJSON(t, r, "GET", "/api/me", "", token)
	if code != http.StatusOK {
		t.Fatalf("me: got %d", code)
	}
	perms, _ := out["permissions"].([]any)
	if len(perms) != len(dsmodel.AllPermissions) {
		t.Fatalf("admin perms: %v", out)
	}

	// 无效 token → 401
	if code, _ := doJSON(t, r, "GET", "/api/me", "", "bad.token.here"); code != http.StatusUnauthorized {
		t.Fatalf("bad token: got %d", code)
	}
}

func TestPermissionEnforcement(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// admin 可访问 /api/users（users.manage）
	if code, _ := doJSON(t, r, "GET", "/api/users", "", token); code != http.StatusOK {
		t.Fatalf("admin list users: got %d", code)
	}

	// admin 创建只读角色 + 只读用户
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"reader","description":"只读","permissions":["rooms.view"]}`, token)
	roleID, _ := out["id"].(float64)
	if roleID == 0 {
		t.Fatalf("create role failed: %v", out)
	}
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"reader1","name":"只读","password":"pass","role_ids":[`+strconv.Itoa(int(roleID))+`]}`, token)
	uid := int(out["id"].(float64))

	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"reader1","password":"pass"}`, "")
	rToken := out["token"].(string)

	// 只读用户访问 /api/users → 403
	if code, _ := doJSON(t, r, "GET", "/api/users", "", rToken); code != http.StatusForbidden {
		t.Fatalf("reader list users: got %d", code)
	}
	// 只读用户浏览候选人 → 200（任意登录）
	if code, _ := doJSON(t, r, "GET", "/api/candidates", "", rToken); code != http.StatusOK {
		t.Fatalf("reader list candidates: got %d", code)
	}
	// 只读用户建候选人（缺 candidates.create）→ 403
	if code, _ := doJSON(t, r, "POST", "/api/candidates", `{"name":"x"}`, rToken); code != http.StatusForbidden {
		t.Fatalf("reader create candidate: got %d", code)
	}
	_ = uid
}

// TestWSMessageAuthAndSync 验证 WS：RESTful 路径 + auth 消息 + 候选人维度同步。
func TestWSMessageAuthAndSync(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 准备：候选人签到 → 建房 → 拉取 → 加入（让房间有候选人与成员）
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024001","name":"张三","profile":"后端"}`, token)
	candID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/candidates/"+itoa(candID)+"/check-in", "", token)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/rooms/"+itoa(roomID)+"/candidate", `{"candidate_id":`+itoa(candID)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	adminID := int(out["user"].(map[string]any)["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/members", `{"user_id":`+itoa(adminID)+`}`, token)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/rooms/" + itoa(roomID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	// 先发 auth
	if err := conn.WriteJSON(map[string]any{"op": "auth", "req_id": "a1", "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	// 读回执（可能先到 member_joined 事件，再 reply）
	gotAuthReply := false
	for i := 0; i < 10 && !gotAuthReply; i++ {
		var env map[string]any
		if err := conn.ReadJSON(&env); err != nil {
			t.Fatalf("read auth env: %v", err)
		}
		if env["type"] == "reply" && env["req_id"] == "a1" {
			if d, _ := env["data"].(map[string]any); d["ok"] != true {
				t.Fatalf("auth reply not ok: %v", env)
			}
			gotAuthReply = true
		}
	}
	if !gotAuthReply {
		t.Fatalf("auth reply not received")
	}

	// 再发 sync，回执应含房间快照与候选人消息
	if err := conn.WriteJSON(map[string]any{"op": "sync", "req_id": "s1", "data": map[string]any{"room_id": roomID, "last_msg_id": 0, "last_seq": 0}}); err != nil {
		t.Fatalf("write sync: %v", err)
	}
	gotSync := false
	for i := 0; i < 10 && !gotSync; i++ {
		var env map[string]any
		if err := conn.ReadJSON(&env); err != nil {
			t.Fatalf("read sync env: %v", err)
		}
		if env["type"] == "reply" && env["req_id"] == "s1" {
			d, _ := env["data"].(map[string]any)
			room, _ := d["room"].(map[string]any)
			cand, _ := room["candidate"].(map[string]any)
			if cand["name"] != "张三" {
				t.Fatalf("sync room candidate mismatch: %v", d)
			}
			gotSync = true
		}
	}
	if !gotSync {
		t.Fatalf("sync reply not received")
	}
}

// TestRoomCompleteClearsCandidateForNext 验证：候选人推进到 COMPLETED 后房间自动清空，
// sync 返回空房快照，且同一房间可继续拉取下一候选人（消息按候选人归档）。
func TestRoomCompleteClearsCandidateForNext(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 准备：候选人 A/B 签到；建房 → 拉取 A → 加入成员
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024002","name":"甲","profile":"后端"}`, token)
	candA := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024003","name":"乙","profile":"前端"}`, token)
	candB := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/candidates/"+itoa(candA)+"/check-in", "", token)
	doJSON(t, r, "PUT", "/api/candidates/"+itoa(candB)+"/check-in", "", token)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/rooms/"+itoa(roomID)+"/candidate", `{"candidate_id":`+itoa(candA)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	adminID := int(out["user"].(map[string]any)["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/members", `{"user_id":`+itoa(adminID)+`}`, token)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/rooms/" + itoa(roomID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	var seenLog []map[string]any
	readUntil := func(pred func(map[string]any) bool) map[string]any {
		t.Helper()
		for i := 0; i < 20; i++ {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			var env map[string]any
			if err := conn.ReadJSON(&env); err != nil {
				t.Fatalf("read ws: %v (log=%v)", err, seenLog)
			}
			seenLog = append(seenLog, env)
			if pred(env) {
				return env
			}
		}
		t.Fatalf("condition not met in read loop (log=%v)", seenLog)
		return nil
	}

	// auth
	if err := conn.WriteJSON(map[string]any{"op": "auth", "req_id": "a1", "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	readUntil(func(env map[string]any) bool {
		return env["type"] == "reply" && env["req_id"] == "a1"
	})

	// sync：确认房间绑定候选人 A
	if err := conn.WriteJSON(map[string]any{"op": "sync", "req_id": "s1", "data": map[string]any{"room_id": roomID, "last_msg_id": 0, "last_seq": 0}}); err != nil {
		t.Fatalf("write sync: %v", err)
	}
	sync1 := readUntil(func(env map[string]any) bool {
		return env["type"] == "reply" && env["req_id"] == "s1"
	})
	d1, _ := sync1["data"].(map[string]any)
	if r1, _ := d1["room"].(map[string]any); r1["candidate"] == nil {
		t.Fatalf("initial sync should have candidate: %v", d1)
	}

	// 推进到 IN_PROGRESS -> COMPLETED，等待完成的 room_phase_changed 事件
	if err := conn.WriteJSON(map[string]any{"op": "move_phase", "req_id": "m1", "data": map[string]any{"room_id": roomID, "to": "IN_PROGRESS"}}); err != nil {
		t.Fatalf("write move in_progress: %v", err)
	}
	readUntil(func(env map[string]any) bool {
		return env["type"] == "reply" && env["req_id"] == "m1"
	})
	if err := conn.WriteJSON(map[string]any{"op": "move_phase", "req_id": "m2", "data": map[string]any{"room_id": roomID, "to": "COMPLETED"}}); err != nil {
		t.Fatalf("write move completed: %v", err)
	}
	readUntil(func(env map[string]any) bool {
		if env["type"] != "room_phase_changed" {
			return false
		}
		chanEv, _ := env["data"].(map[string]any)
		payload, _ := chanEv["data"].(map[string]any)
		return payload["To"] == "COMPLETED"
	})
	readUntil(func(env map[string]any) bool {
		return env["type"] == "reply" && env["req_id"] == "m2"
	})

	// 完成后 sync：房间快照应无候选人，消息为空（A 的消息归档在候选人处）
	if err := conn.WriteJSON(map[string]any{"op": "sync", "req_id": "s2", "data": map[string]any{"room_id": roomID, "last_msg_id": 0, "last_seq": 0}}); err != nil {
		t.Fatalf("write sync2: %v", err)
	}
	sync2 := readUntil(func(env map[string]any) bool {
		return env["type"] == "reply" && env["req_id"] == "s2"
	})
	d2, _ := sync2["data"].(map[string]any)
	if r2, _ := d2["room"].(map[string]any); r2["candidate"] != nil {
		t.Fatalf("room should be empty after complete: %v", d2)
	}
	if msgs, ok := d2["messages"].([]any); ok && len(msgs) != 0 {
		t.Fatalf("room messages should be empty after complete: %v", d2)
	}

	// 同一房间拉取下一候选人 B
	code, pullOut := doJSON(t, r, "PUT", "/api/rooms/"+itoa(roomID)+"/candidate", `{"candidate_id":`+itoa(candB)+`}`, token)
	if code != http.StatusOK {
		t.Fatalf("pull B: got %d %v", code, pullOut)
	}
	code, roomOut := doJSON(t, r, "GET", "/api/rooms/"+itoa(roomID), "", token)
	if code != http.StatusOK {
		t.Fatalf("get room: got %d", code)
	}
	if rm, _ := roomOut["candidate"].(map[string]any); rm == nil || int(rm["id"].(float64)) != candB {
		t.Fatalf("room should bind candidate B: %v", roomOut)
	}
}

// TestSignedInBroadcastsToAllRooms 验证候选人签到事件经全局广播推送至所有开放房间，
// 使各房间"待分配池"可实时更新。
func TestSignedInBroadcastsToAllRooms(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 建两个面试官用户（各持 interviewer 权限，含 rooms.chat），分别进不同房间，
	// 以满足"一用户至多一活跃房间"约束。
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"itv","description":"面试官","permissions":["rooms.view","rooms.chat","candidates.create","candidates.checkin","candidates.assign"]}`, token)
	roleID := int(out["id"].(float64))
	userIDs := []int{}
	for _, u := range []string{"itvA", "itvB"} {
		_, out = doJSON(t, r, "POST", "/api/users", `{"username":"`+u+`","name":"面试官","password":"pass","role_ids":[`+itoa(roleID)+`]}`, token)
		userIDs = append(userIDs, int(out["id"].(float64)))
	}

	// 建两个房间并各加入一位面试官
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomA := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomA)+"/members", `{"user_id":`+itoa(userIDs[0])+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomB := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomB)+"/members", `{"user_id":`+itoa(userIDs[1])+`}`, token)

	// 为两用户签发各自 token
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"itvA","password":"pass"}`, "")
	tokenA := out["token"].(string)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"itvB","password":"pass"}`, "")
	tokenB := out["token"].(string)

	srv := httptest.NewServer(r)
	defer srv.Close()

	dial := func(room int) *websocket.Conn {
		t.Helper()
		url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/rooms/" + itoa(room)
		conn, _, err := websocket.DefaultDialer.Dial(url, nil)
		if err != nil {
			t.Fatalf("dial room %d: %v", room, err)
		}
		return conn
	}
	readReply := func(conn *websocket.Conn, reqID string) {
		t.Helper()
		for i := 0; i < 20; i++ {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			var env map[string]any
			if err := conn.ReadJSON(&env); err != nil {
				t.Fatalf("read: %v", err)
			}
			if env["type"] == "reply" && env["req_id"] == reqID {
				return
			}
		}
		t.Fatalf("reply %s not received", reqID)
	}

	connA := dial(roomA)
	defer connA.Close()
	connB := dial(roomB)
	defer connB.Close()

	// 两连接各 auth
	for _, c := range []struct {
		conn *websocket.Conn
		id   string
		tok  string
	}{{connA, "a1", tokenA}, {connB, "b1", tokenB}} {
		if err := c.conn.WriteJSON(map[string]any{"op": "auth", "req_id": c.id, "data": map[string]string{"token": c.tok}}); err != nil {
			t.Fatalf("write auth: %v", err)
		}
		readReply(c.conn, c.id)
	}

	// 建候选人并签到 → 应全局广播到房间 B
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024004","name":"新签","profile":"前端"}`, token)
	candID := int(out["id"].(float64))
	if code, _ := doJSON(t, r, "PUT", "/api/candidates/"+itoa(candID)+"/check-in", "", token); code != http.StatusOK {
		t.Fatalf("checkin failed: %d", code)
	}

	// 房间 B 应收到 candidate_signed_in 事件（全局广播跨房间）
	for i := 0; i < 20; i++ {
		_ = connB.SetReadDeadline(time.Now().Add(2 * time.Second))
		var env map[string]any
		if err := connB.ReadJSON(&env); err != nil {
			t.Fatalf("read connB: %v", err)
		}
		if env["type"] == "candidate_signed_in" {
			return
		}
	}
	t.Fatalf("room B should receive candidate_signed_in global event")
}

// TestCandidateTranscriptArchivedAfterComplete 验证：候选人完成面试后房间虽已解绑，
// 其面试过程消息仍按候选人归档，可经 GET /api/candidates/:id/messages 回看；
// 查看与管理分离 —— 归档对任意登录用户可读（无权限的普通用户也可查看）。
func TestCandidateTranscriptArchivedAfterComplete(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 准备：候选人签到 → 建房 → 拉取 → 加入成员
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024002","name":"甲","profile":"后端"}`, token)
	candID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/candidates/"+itoa(candID)+"/check-in", "", token)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/rooms/"+itoa(roomID)+"/candidate", `{"candidate_id":`+itoa(candID)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	adminID := int(out["user"].(map[string]any)["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/members", `{"user_id":`+itoa(adminID)+`}`, token)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/rooms/" + itoa(roomID)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	readReply := func(reqID string) map[string]any {
		t.Helper()
		for i := 0; i < 20; i++ {
			_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			var env map[string]any
			if err := conn.ReadJSON(&env); err != nil {
				t.Fatalf("read ws: %v", err)
			}
			if env["type"] == "reply" && env["req_id"] == reqID {
				return env
			}
		}
		t.Fatalf("reply %s not received", reqID)
		return nil
	}
	sendOp := func(reqID, op string, data map[string]any) map[string]any {
		t.Helper()
		if data == nil {
			data = map[string]any{}
		}
		if err := conn.WriteJSON(map[string]any{"op": op, "req_id": reqID, "data": data}); err != nil {
			t.Fatalf("write %s: %v", op, err)
		}
		return readReply(reqID)
	}

	// auth → 推进 IN_PROGRESS → 发两条记录 → 推进 COMPLETED
	if d := sendOp("a1", "auth", map[string]any{"token": token}); d == nil {
		t.Fatalf("unreachable")
	} else if ok, _ := d["data"].(map[string]any)["ok"].(bool); !ok {
		t.Fatalf("auth failed: %v", d)
	}
	sendOp("m1", "move_phase", map[string]any{"to": "IN_PROGRESS"})
	sendOp("s1", "send_msg", map[string]any{"content": "自我介绍与项目经历"})
	sendOp("s2", "send_msg", map[string]any{"content": "算法题通过"})
	sendOp("m2", "move_phase", map[string]any{"to": "COMPLETED"})

	// 完成后：归档接口返回全过程消息（按 id 升序，含发送者资料）
	code, out := doJSON(t, r, "GET", "/api/candidates/"+itoa(candID)+"/messages", "", token)
	if code != http.StatusOK {
		t.Fatalf("get transcript: got %d %v", code, out)
	}
	items, _ := out["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("transcript should have 2 messages: %v", out)
	}
	first, _ := items[0].(map[string]any)
	if first["content"] != "自我介绍与项目经历" {
		t.Fatalf("transcript order/content mismatch: %v", items)
	}
	sender, _ := first["sender"].(map[string]any)
	if sender == nil || sender["username"] != "admin" {
		t.Fatalf("sender should be preloaded: %v", first)
	}

	// 只读角色（rooms.view）可查看归档
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"viewer","description":"","permissions":["rooms.view"]}`, token)
	viewerRole := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"viewer1","name":"","password":"pass","role_ids":[`+itoa(viewerRole)+`]}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"viewer1","password":"pass"}`, "")
	vToken := out["token"].(string)
	if code, _ := doJSON(t, r, "GET", "/api/candidates/"+itoa(candID)+"/messages", "", vToken); code != http.StatusOK {
		t.Fatalf("viewer transcript: got %d", code)
	}

	// 无任何权限的普通用户也可查看归档（查看与管理分离）
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"nobody","name":"","password":"pass","role_ids":[]}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"nobody","password":"pass"}`, "")
	nToken := out["token"].(string)
	if code, _ := doJSON(t, r, "GET", "/api/candidates/"+itoa(candID)+"/messages", "", nToken); code != http.StatusOK {
		t.Fatalf("no-perm transcript: got %d", code)
	}

	// 不存在的候选人 → 404
	if code, _ := doJSON(t, r, "GET", "/api/candidates/9999/messages", "", token); code != http.StatusNotFound {
		t.Fatalf("missing candidate transcript: got %d", code)
	}
}

// TestDepartmentManageAndPermission 部门管理接口：admin 可 CRUD，无 users.manage 权限者被拒。
func TestDepartmentManageAndPermission(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 管理员为全局系统角色，不隶属任何部门（种子不预置部门）
	_, out = doJSON(t, r, "GET", "/api/me", "", token)
	if user, _ := out["user"].(map[string]any); user["department"] != nil {
		t.Fatalf("admin should not have a department: %v", out)
	}

	// 创建部门（含预期人数）
	code, out := doJSON(t, r, "POST", "/api/departments", `{"name":"前端组","description":"负责前端岗位","expected_count":15}`, token)
	if code != http.StatusCreated {
		t.Fatalf("create department: got %d %v", code, out)
	}
	deptID := int(out["id"].(float64))

	// 列出部门
	code, out = doJSON(t, r, "GET", "/api/departments", "", token)
	if code != http.StatusOK {
		t.Fatalf("list departments: got %d", code)
	}
	items, _ := out["items"].([]any)
	found := false
	for _, it := range items {
		if d, _ := it.(map[string]any); int(d["id"].(float64)) == deptID && d["name"] == "前端组" {
			found = true
			if int(d["expected_count"].(float64)) != 15 {
				t.Fatalf("expected_count: got %v, want 15", d["expected_count"])
			}
		}
	}
	if !found {
		t.Fatalf("created department not listed: %v", out)
	}

	// 更新部门（含预期人数）
	code, out = doJSON(t, r, "PUT", "/api/departments/"+itoa(deptID), `{"name":"前端组","description":"负责前端岗位","expected_count":25}`, token)
	if code != http.StatusOK {
		t.Fatalf("update department: got %d %v", code, out)
	}
	code, out = doJSON(t, r, "GET", "/api/departments", "", token)
	items, _ = out["items"].([]any)
	for _, it := range items {
		if d, _ := it.(map[string]any); int(d["id"].(float64)) == deptID && int(d["expected_count"].(float64)) != 25 {
			t.Fatalf("expected_count after update: got %v, want 25", d["expected_count"])
		}
	}

	// 负数预期人数 → 400
	if code, _ := doJSON(t, r, "PUT", "/api/departments/"+itoa(deptID), `{"name":"前端组","description":"","expected_count":-1}`, token); code != http.StatusBadRequest {
		t.Fatalf("negative expected_count: got %d", code)
	}

	// 创建用户并归属部门
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"fe1","name":"前端一","password":"pass","role_ids":[],"department_id":`+itoa(deptID)+`}`, token)
	uid := int(out["id"].(float64))
	code, out = doJSON(t, r, "GET", "/api/users", "", token)
	if code != http.StatusOK {
		t.Fatalf("list users: got %d", code)
	}
	users, _ := out["items"].([]any)
	userFound := false
	for _, it := range users {
		if u, _ := it.(map[string]any); int(u["id"].(float64)) == uid {
			if d, _ := u["department"].(map[string]any); d == nil || int(d["id"].(float64)) != deptID {
				t.Fatalf("user department not filled: %v", u)
			}
			userFound = true
		}
	}
	if !userFound {
		t.Fatalf("created user not listed: %v", out)
	}

	// 删除被引用部门 → 400
	code, out = doJSON(t, r, "DELETE", "/api/departments/"+itoa(deptID), "", token)
	if code != http.StatusBadRequest {
		t.Fatalf("delete in-use department: got %d %v", code, out)
	}

	// 无 users.manage 权限用户（interviewer 角色）：可读部门（志愿选择器用），但不可增删改
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	code, itvRole := doJSON(t, r, "POST", "/api/roles", `{"name":"itv2","description":"","permissions":["rooms.view"]}`, token)
	if code != http.StatusCreated {
		t.Fatalf("create role: got %d %v", code, itvRole)
	}
	roleID := int(itvRole["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"itv2u","name":"","password":"pass","role_ids":[`+itoa(roleID)+`]}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"itv2u","password":"pass"}`, "")
	tokenB := out["token"].(string)
	if code, got := doJSON(t, r, "GET", "/api/departments", "", tokenB); code != http.StatusOK {
		t.Fatalf("interviewer list departments: got %d %v", code, got)
	}
	if code, _ := doJSON(t, r, "POST", "/api/departments", `{"name":"越权组"}`, tokenB); code != http.StatusForbidden {
		t.Fatalf("interviewer create department: got %d", code)
	}
	if code, _ := doJSON(t, r, "DELETE", "/api/departments/"+itoa(deptID), "", tokenB); code != http.StatusForbidden {
		t.Fatalf("interviewer delete department: got %d", code)
	}
}

// TestSystemStatusManageAndPermission 系统状态接口：admin 可读可切换，无权限者被拒。
func TestSystemStatusManageAndPermission(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 默认面试阶段
	code, out := doJSON(t, r, "GET", "/api/system/status", "", token)
	if code != http.StatusOK {
		t.Fatalf("get status: got %d %v", code, out)
	}
	if out["phase"] != "interview" {
		t.Fatalf("default phase: %v", out)
	}

	// 切换到录取阶段
	code, out = doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"admission"}`, token)
	if code != http.StatusOK {
		t.Fatalf("set admission: got %d %v", code, out)
	}
	code, out = doJSON(t, r, "GET", "/api/system/status", "", token)
	if out["phase"] != "admission" {
		t.Fatalf("phase after set: %v", out)
	}

	// 切换到捡漏阶段
	code, out = doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"leftover"}`, token)
	if code != http.StatusOK {
		t.Fatalf("set leftover: got %d %v", code, out)
	}
	code, out = doJSON(t, r, "GET", "/api/system/status", "", token)
	if out["phase"] != "leftover" {
		t.Fatalf("phase after set leftover: %v", out)
	}

	// 非法阶段 → 400
	if code, _ := doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"bogus"}`, token); code != http.StatusBadRequest {
		t.Fatalf("invalid phase: got %d", code)
	}

	// 合并更新：单独改出价步长（资源字段名 bid_step），阶段不受影响
	code, out = doJSON(t, r, "PATCH", "/api/system/status", `{"bid_step":50}`, token)
	if code != http.StatusOK {
		t.Fatalf("set bid step: got %d %v", code, out)
	}
	code, out = doJSON(t, r, "GET", "/api/system/status", "", token)
	if int(out["bid_step"].(float64)) != 50 || out["phase"] != "leftover" {
		t.Fatalf("status after bid step: %v", out)
	}

	// 两字段皆缺 → 400
	if code, _ := doJSON(t, r, "PATCH", "/api/system/status", `{}`, token); code != http.StatusBadRequest {
		t.Fatalf("empty patch: got %d", code)
	}

	// 无 users.manage 权限用户：可读状态（UI 全员可见），但不可切换
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"itv3","description":"","permissions":["rooms.view"]}`, token)
	roleID := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"itv3u","name":"","password":"pass","role_ids":[`+itoa(roleID)+`]}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"itv3u","password":"pass"}`, "")
	tokenB := out["token"].(string)
	if code, _ := doJSON(t, r, "GET", "/api/system/status", "", tokenB); code != http.StatusOK {
		t.Fatalf("interviewer get status: got %d", code)
	}
	if code, _ := doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"admission"}`, tokenB); code != http.StatusForbidden {
		t.Fatalf("interviewer set status: got %d", code)
	}
}

// TestCandidateAdmissionByDepartmentAndPermission 录取决定按部门隔离：
// 默认只能看本部门记录；admin（browse_all）可跨部门；记录需 admissions.record。
func TestCandidateAdmissionByDepartmentAndPermission(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 建两个部门
	_, out = doJSON(t, r, "POST", "/api/departments", `{"name":"后端组","description":""}`, token)
	deptA := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/departments", `{"name":"前端组","description":""}`, token)
	deptB := int(out["id"].(float64))

	// 建两名面试官：一名归后端组（可记录录取决定）、一名归前端组（只读）
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"feAdmin","name":"前端管理员","password":"pass","role_ids":[],"department_id":`+itoa(deptA)+`}`, token)
	uidA := int(out["id"].(float64))
	// 给 uidA 授 admissions.record 与 rooms.view（可记录录取决定）
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"feMgr","description":"","permissions":["admissions.record","rooms.view","candidates.create","candidates.checkin"]}`, token)
	roleMgr := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/users/"+itoa(uidA), `{"username":"feAdmin","name":"前端管理员","role_ids":[`+itoa(roleMgr)+`],"department_id":`+itoa(deptA)+`}`, token)

	// 只读面试官（无 admissions.record）归前端组
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"feReader","name":"前端只读","password":"pass","role_ids":[],"department_id":`+itoa(deptB)+`}`, token)
	uidB := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"reader2","description":"","permissions":["rooms.view"]}`, token)
	roleReader := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/users/"+itoa(uidB), `{"username":"feReader","name":"前端只读","role_ids":[`+itoa(roleReader)+`],"department_id":`+itoa(deptB)+`}`, token)

	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"feAdmin","password":"pass"}`, "")
	tokenA := out["token"].(string)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"feReader","password":"pass"}`, "")
	tokenB := out["token"].(string)

	// 建候选人
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024001","name":"张三","profile":"后端"}`, token)
	candID := int(out["id"].(float64))

	// feAdmin（admissions.record）记录本部门录取决定 → 200
	code, out := doJSON(t, r, "PUT", "/api/admissions/"+itoa(candID), `{"status":"admitted"}`, tokenA)
	if code != http.StatusOK {
		t.Fatalf("upsert own dept: got %d %v", code, out)
	}

	// feReader 无 admissions.record → 403
	if code, _ := doJSON(t, r, "PUT", "/api/admissions/"+itoa(candID), `{"status":"withdrawn"}`, tokenB); code != http.StatusForbidden {
		t.Fatalf("reader upsert: got %d", code)
	}

	// feReader（无 browse_all）看录取 → 只见本部门（空）
	code, out = doJSON(t, r, "GET", "/api/admissions", "", tokenB)
	if code != http.StatusOK {
		t.Fatalf("reader list: got %d", code)
	}
	if items, _ := out["items"].([]any); len(items) != 0 {
		t.Fatalf("reader should see own dept only (empty): %v", out)
	}

	// feAdmin（无 browse_all）看录取 → 只见本部门（一条）
	code, out = doJSON(t, r, "GET", "/api/admissions", "", tokenA)
	if code != http.StatusOK {
		t.Fatalf("feAdmin list: got %d", code)
	}
	itemsA, _ := out["items"].([]any)
	if len(itemsA) != 1 {
		t.Fatalf("feAdmin should see 1 own-dept record: %v", out)
	}

	// admin（browse_all，admin 预置全部权限）看录取 → 跨部门（一条）
	code, out = doJSON(t, r, "GET", "/api/admissions", "", token)
	if code != http.StatusOK {
		t.Fatalf("admin list: got %d", code)
	}
	itemsAdmin, _ := out["items"].([]any)
	if len(itemsAdmin) != 1 {
		t.Fatalf("admin should see all dept records: %v", out)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// boardAuth 向看板连接发送 auth 并等待回执；返回回执是否 ok。
func boardAuth(t *testing.T, conn *websocket.Conn, reqID, token string) bool {
	t.Helper()
	if err := conn.WriteJSON(map[string]any{"op": "auth", "req_id": reqID, "data": map[string]string{"token": token}}); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	var env map[string]any
	if err := conn.ReadJSON(&env); err != nil {
		t.Fatalf("read auth reply: %v", err)
	}
	d, _ := env["data"].(map[string]any)
	return env["type"] == "reply" && env["req_id"] == reqID && d["ok"] == true
}

// readEventsUntil 在看板连接上读取事件，直到看到全部期望的事件类型（跳过 reply），
// 返回各事件类型的出现次数（供重复投递断言）。
func readEventsUntil(t *testing.T, conn *websocket.Conn, want ...string) map[string]int {
	t.Helper()
	remaining := map[string]bool{}
	for _, w := range want {
		remaining[w] = true
	}
	counts := map[string]int{}
	for len(remaining) > 0 {
		var env map[string]any
		if err := conn.ReadJSON(&env); err != nil {
			t.Fatalf("read event (waiting %v): %v", remaining, err)
		}
		typ, _ := env["type"].(string)
		if typ == "reply" {
			continue
		}
		counts[typ]++
		delete(remaining, typ)
	}
	return counts
}

// TestBoardChannelAuthAndEvents 验证看板通道：rooms.view 权限门槛，
// 以及候选人/房间 CRUD 与签到等全局、房间级事件均扇出到看板订阅者。
func TestBoardChannelAuthAndEvents(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 无 rooms.view 的用户：鉴权应被拒
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"noview","description":"无查看","permissions":["candidates.create"]}`, token)
	roleID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/users", `{"username":"noview1","name":"无权","password":"pass","role_ids":[`+itoa(roleID)+`]}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"noview1","password":"pass"}`, "")
	noViewToken, _ := out["token"].(string)

	srv := httptest.NewServer(r)
	defer srv.Close()

	boardURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/board"

	// 无权限用户：auth 回执不 ok，随后连接被服务端关闭
	denyConn, _, err := websocket.DefaultDialer.Dial(boardURL, nil)
	if err != nil {
		t.Fatalf("dial deny: %v", err)
	}
	if err := denyConn.WriteJSON(map[string]any{"op": "auth", "req_id": "d1", "data": map[string]string{"token": noViewToken}}); err != nil {
		t.Fatalf("write deny auth: %v", err)
	}
	var env map[string]any
	if err := denyConn.ReadJSON(&env); err != nil {
		t.Fatalf("read deny reply: %v", err)
	}
	if d, _ := env["data"].(map[string]any); d["ok"] == true {
		t.Fatalf("board auth should be denied for user without rooms.view")
	}
	denyConn.Close()

	// admin：鉴权成功后接收业务事件
	conn, _, err := websocket.DefaultDialer.Dial(boardURL, nil)
	if err != nil {
		t.Fatalf("dial board: %v", err)
	}
	defer conn.Close()
	if !boardAuth(t, conn, "b1", token) {
		t.Fatalf("board auth failed")
	}

	// 同时开一条房间通道连接：验证全局事件不会因"房间扇出 + 全局扇出"重复投递给看板
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	warmRoomID := int(out["id"].(float64))
	roomURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/rooms/" + itoa(warmRoomID)
	roomConn, _, err := websocket.DefaultDialer.Dial(roomURL, nil)
	if err != nil {
		t.Fatalf("dial warm room: %v", err)
	}
	defer roomConn.Close()
	if !boardAuth(t, roomConn, "r1", token) {
		t.Fatalf("room auth failed")
	}

	// 触发：建候选人 → 建房 → 签到 → 拉取（房间级事件也应扇出）
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024001","name":"张三","profile":"后端"}`, token)
	candID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms", "", token)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "PUT", "/api/candidates/"+itoa(candID)+"/check-in", "", token)
	doJSON(t, r, "PUT", "/api/rooms/"+itoa(roomID)+"/candidate", `{"candidate_id":`+itoa(candID)+`}`, token)

	counts := readEventsUntil(t, conn,
		"candidate_created", "room_created", "candidate_signed_in", "candidate_assigned")
	if counts["candidate_signed_in"] != 1 || counts["candidate_assigned"] != 1 {
		t.Fatalf("duplicate delivery to board: %v", counts)
	}

	// 删除空房 → room_deleted；编辑/删除候选人 → candidate_updated / candidate_deleted
	doJSON(t, r, "PUT", "/api/candidates/"+itoa(candID), `{"student_no":"2024001","name":"张三丰","profile":""}`, token)
	doJSON(t, r, "DELETE", "/api/candidates/"+itoa(candID), "", token)
	readEventsUntil(t, conn, "candidate_updated", "candidate_deleted")
}

// TestLeftoverBiddingEndpoints 捡漏竞拍 HTTP 契约：
// 出价需 admissions.record 且仅捡漏阶段；预算约束；出价保密为本部门；
// 手动结算入口已移除（404），改由 users.manage 切到结算阶段自动结算。
func TestLeftoverBiddingEndpoints(t *testing.T) {
	r := newTestApp(t)

	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 两个部门：A 预期 20 人（预算 2000），B 预期 0（预算下限 500）
	_, out = doJSON(t, r, "POST", "/api/departments", `{"name":"A组","expected_count":20}`, token)
	deptA := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/departments", `{"name":"B组","expected_count":0}`, token)
	deptB := int(out["id"].(float64))

	// 出价角色 + 两名分属两部门的出价人
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"bidder","description":"","permissions":["admissions.record","rooms.view"]}`, token)
	roleBid := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"bidA","name":"bidA","password":"pass","role_ids":[`+itoa(roleBid)+`],"department_id":`+itoa(deptA)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"bidB","name":"bidB","password":"pass","role_ids":[`+itoa(roleBid)+`],"department_id":`+itoa(deptB)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"bidA","password":"pass"}`, "")
	tokenA := out["token"].(string)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"bidB","password":"pass"}`, "")
	tokenB := out["token"].(string)

	// 候选人 + 切到捡漏阶段
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024005","name":"张三"}`, token)
	cand := int(out["id"].(float64))
	if code, _ := doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"leftover"}`, token); code != http.StatusOK {
		t.Fatalf("set phase failed")
	}

	// A 出 800 → 200；B 预算 500 出 600 → 400 budget_exceeded
	if code, out := doJSON(t, r, "PUT", "/api/leftover/bids/"+itoa(cand), `{"amount":800}`, tokenA); code != http.StatusOK {
		t.Fatalf("bidA: got %d %v", code, out)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/leftover/bids/"+itoa(cand), `{"amount":600}`, tokenB); code != http.StatusBadRequest {
		t.Fatalf("bidB over budget: got %d", code)
	}

	// 出价保密：B 只能看到自己的出价（此处为空）
	code, out := doJSON(t, r, "GET", "/api/leftover/bids", "", tokenB)
	if code != http.StatusOK {
		t.Fatalf("bids B: got %d", code)
	}
	if items, _ := out["items"].([]any); len(items) != 0 {
		t.Fatalf("B must not see A's bid: %v", out)
	}

	// 总览：A 的 my 预算 2000/已出 800/剩 1200；B 部门 spent 保密为 null
	code, out = doJSON(t, r, "GET", "/api/leftover", "", tokenA)
	if code != http.StatusOK {
		t.Fatalf("overview: got %d", code)
	}
	my := out["my"].(map[string]any)
	if my["budget"].(float64) != 2000 || my["spent"].(float64) != 800 || my["remaining"].(float64) != 1200 {
		t.Fatalf("my=%v", my)
	}
	for _, d := range out["departments"].([]any) {
		dm := d.(map[string]any)
		if int(dm["id"].(float64)) == deptB && dm["spent"] != nil {
			t.Fatalf("deptB spent leaked: %v", dm)
		}
	}

	// 管理端（browse_all）：跨部门出价与全部部门 spent 可见
	code, out = doJSON(t, r, "GET", "/api/leftover/bids", "", token)
	if code != http.StatusOK {
		t.Fatalf("admin bids: got %d", code)
	}
	all := out["items"].([]any)
	if len(all) != 1 {
		t.Fatalf("admin should see all dept bids: %v", out)
	}
	if b := all[0].(map[string]any); int(b["department_id"].(float64)) != deptA || b["amount"].(float64) != 800 {
		t.Fatalf("admin bids[0]=%v", b)
	}
	code, out = doJSON(t, r, "GET", "/api/leftover", "", token)
	if code != http.StatusOK {
		t.Fatalf("admin overview: got %d", code)
	}
	if out["my"] != nil {
		t.Fatalf("admin has no dept, my must be nil: %v", out["my"])
	}
	for _, d := range out["departments"].([]any) {
		dm := d.(map[string]any)
		if dm["spent"] == nil {
			t.Fatalf("admin must see spent for all depts: %v", dm)
		}
		if int(dm["id"].(float64)) == deptA && dm["spent"].(float64) != 800 {
			t.Fatalf("deptA spent=%v", dm["spent"])
		}
	}

	// 手动结算入口已移除 → 404
	if code, _ := doJSON(t, r, "POST", "/api/leftover/results", `{"candidate_id":`+itoa(cand)+`}`, token); code != http.StatusNotFound {
		t.Fatalf("manual resolve route must be gone: got %d", code)
	}
	// 非 users.manage 切到结算阶段 → 403
	if code, _ := doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"settlement"}`, tokenA); code != http.StatusForbidden {
		t.Fatalf("bidder settlement switch: got %d", code)
	}
	// admin 切到结算阶段 → 自动按出价结算（A 800 胜出）
	if code, out := doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"settlement"}`, token); code != http.StatusOK {
		t.Fatalf("set settlement: got %d %v", code, out)
	}

	// 结算后封盘（竞拍只读）；结果公开
	if code, _ := doJSON(t, r, "PUT", "/api/leftover/bids/"+itoa(cand), `{"amount":100}`, tokenB); code != http.StatusBadRequest {
		t.Fatalf("bid after settlement: got %d", code)
	}
	code, out = doJSON(t, r, "GET", "/api/leftover/results", "", tokenB)
	if code != http.StatusOK {
		t.Fatalf("results: got %d", code)
	}
	items, _ := out["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("results=%v", out)
	}
	if it := items[0].(map[string]any); int(it["department_id"].(float64)) != deptA || it["amount"].(float64) != 800 {
		t.Fatalf("results[0]=%v", it)
	}
}

// TestRoomNamingREST 房间命名往返：创建带名（裁剪空白）、PATCH 改名与清除、
// 超长 400、无体旧调用兼容（未命名）。
func TestRoomNamingREST(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	_, out := doJSON(t, r, "POST", "/api/rooms", `{"name":" 终面间 "}`, token)
	id := int(out["id"].(float64))
	if id == 0 {
		t.Fatalf("create: %v", out)
	}
	if code, got := doJSON(t, r, "GET", "/api/rooms/"+itoa(id), "", token); code != http.StatusOK || got["name"] != "终面间" {
		t.Fatalf("创建带名: %d %v", code, got)
	}

	if code, _ := doJSON(t, r, "PATCH", "/api/rooms/"+itoa(id), `{"name":"初面间"}`, token); code != http.StatusOK {
		t.Fatalf("改名应 200: %d", code)
	}
	if _, got := doJSON(t, r, "GET", "/api/rooms/"+itoa(id), "", token); got["name"] != "初面间" {
		t.Fatalf("改名未生效: %v", got)
	}
	if code, _ := doJSON(t, r, "PATCH", "/api/rooms/"+itoa(id), `{"name":""}`, token); code != http.StatusOK {
		t.Fatalf("清除命名应 200: %d", code)
	}
	if _, got := doJSON(t, r, "GET", "/api/rooms/"+itoa(id), "", token); got["name"] != "" {
		t.Fatalf("清除未生效: %v", got)
	}

	if code, _ := doJSON(t, r, "PATCH", "/api/rooms/"+itoa(id), `{"name":"`+strings.Repeat("名", 65)+`"}`, token); code != http.StatusBadRequest {
		t.Fatalf("超长名应 400: %d", code)
	}
	// 无体创建（兼容旧调用）→ 未命名
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	id2 := int(out["id"].(float64))
	if code, got := doJSON(t, r, "GET", "/api/rooms/"+itoa(id2), "", token); code != http.StatusOK || got["name"] != "" {
		t.Fatalf("无体创建应得未命名房: %d %v", code, got)
	}
}

// 面试计时字段经 REST 暴露：进入面试中打点、面试结束后清空（前端据此计时）。
func TestInterviewStartedAtREST(t *testing.T) {
	r := newTestApp(t)
	token := adminToken(t, r)

	_, created := doJSON(t, r, "POST", "/api/candidates", `{"student_no":"700001","name":"计时"}`, token)
	cid := int(created["id"].(float64))
	if cid == 0 {
		t.Fatalf("创建候选人失败: %v", created)
	}
	candPath := "/api/candidates/" + itoa(cid)
	if code, got := doJSON(t, r, "GET", candPath, "", token); code != http.StatusOK || got["interview_started_at"] != nil {
		t.Fatalf("初始应为空: %d %v", code, got["interview_started_at"])
	}
	if code, _ := doJSON(t, r, "PUT", candPath+"/check-in", "", token); code != http.StatusOK {
		t.Fatalf("签到应 200: %d", code)
	}
	_, room := doJSON(t, r, "POST", "/api/rooms", `{"name":""}`, token)
	rid := int(room["id"].(float64))
	if code, _ := doJSON(t, r, "PUT", "/api/rooms/"+itoa(rid)+"/candidate", `{"candidate_id":`+itoa(cid)+`}`, token); code != http.StatusOK {
		t.Fatalf("拉取应 200: %d", code)
	}
	if code, got := doJSON(t, r, "PUT", candPath+"/status", `{"status":"IN_PROGRESS"}`, token); code != http.StatusOK {
		t.Fatalf("进入面试中应 200: %d %v", code, got)
	}
	if _, got := doJSON(t, r, "GET", candPath, "", token); got["interview_started_at"] == nil {
		t.Fatalf("进入面试中应打点: %v", got)
	}
	if code, _ := doJSON(t, r, "PUT", candPath+"/status", `{"status":"COMPLETED"}`, token); code != http.StatusOK {
		t.Fatalf("完成应 200: %d", code)
	}
	if _, done := doJSON(t, r, "GET", candPath, "", token); done["interview_started_at"] != nil {
		t.Fatalf("面试结束后应清空: %v", done["interview_started_at"])
	}
}

// 志愿与调剂接口：interviewer 预置角色默认持有 candidates.preferences；
// 无该权限的角色被拒（403），且该接口无法改动其他资料字段。
func TestCandidatePreferencesREST(t *testing.T) {
	r := newTestApp(t)
	admin := adminToken(t, r)

	_, created := doJSON(t, r, "POST", "/api/candidates",
		`{"student_no":"700101","name":"志愿","phone":"13800000000","first_choice":"技术部","accept_adjust":true}`, admin)
	cid := int(created["id"].(float64))
	candPath := "/api/candidates/" + itoa(cid)

	// interviewer 预置角色应默认带该权限
	_, roles := doJSON(t, r, "GET", "/api/roles", "", admin)
	itvRole := 0
	for _, raw := range roles["items"].([]any) {
		role := raw.(map[string]any)
		if role["name"] != "interviewer" {
			continue
		}
		itvRole = int(role["id"].(float64))
		has := false
		for _, p := range role["permissions"].([]any) {
			perm := p.(map[string]any)
			if perm["permission"] == dsmodel.PermCandidatesPreferences {
				has = true
			}
		}
		if !has {
			t.Fatalf("interviewer 角色缺少 %s：%v", dsmodel.PermCandidatesPreferences, role["permissions"])
		}
	}
	if itvRole == 0 {
		t.Fatal("未找到 interviewer 角色")
	}

	_, u := doJSON(t, r, "POST", "/api/users",
		`{"username":"itv1","name":"面试官","password":"pass","role_ids":[`+itoa(itvRole)+`]}`, admin)
	if int(u["id"].(float64)) == 0 {
		t.Fatalf("创建面试官失败: %v", u)
	}
	_, sess := doJSON(t, r, "POST", "/api/sessions", `{"username":"itv1","password":"pass"}`, "")
	itv, _ := sess["token"].(string)
	if itv == "" {
		t.Fatalf("面试官登录失败: %v", sess)
	}

	if code, got := doJSON(t, r, "PATCH", candPath+"/preferences",
		`{"first_choice":"数字媒体中心","second_choice":"技术保障中心","accept_adjust":false}`, itv); code != http.StatusOK {
		t.Fatalf("面试官改志愿应 200: %d %v", code, got)
	}
	_, after := doJSON(t, r, "GET", candPath, "", itv)
	if after["first_choice"] != "数字媒体中心" || after["second_choice"] != "技术保障中心" || after["accept_adjust"] != false {
		t.Fatalf("志愿/调剂未生效: %v", after)
	}
	if after["name"] != "志愿" || after["phone"] != "13800000000" {
		t.Fatalf("其他资料被改动: %v", after)
	}
	// 面试官无 candidates.manage：整份资料编辑仍应被拒
	if code, _ := doJSON(t, r, "PUT", candPath, `{"student_no":"700101","name":"改名"}`, itv); code != http.StatusForbidden {
		t.Fatalf("面试官改资料应 403: %d", code)
	}

	// 无该权限的角色 → 403
	_, roleOut := doJSON(t, r, "POST", "/api/roles", `{"name":"reader2","description":"","permissions":["rooms.view"]}`, admin)
	rid := int(roleOut["id"].(float64))
	doJSON(t, r, "POST", "/api/users", `{"username":"reader2","name":"只读","password":"pass","role_ids":[`+itoa(rid)+`]}`, admin)
	_, rs := doJSON(t, r, "POST", "/api/sessions", `{"username":"reader2","password":"pass"}`, "")
	if code, _ := doJSON(t, r, "PATCH", candPath+"/preferences", `{"first_choice":"x"}`, rs["token"].(string)); code != http.StatusForbidden {
		t.Fatalf("无权限应 403: %d", code)
	}

	// 未知候选人 → 404
	if code, _ := doJSON(t, r, "PATCH", "/api/candidates/999999/preferences", `{"first_choice":"x"}`, admin); code != http.StatusNotFound {
		t.Fatalf("未知候选人应 404: %d", code)
	}
}

// 出价 0 的 HTTP 契约：0 合法（200 且列表可见 amount=0，重复提交仍是同一条），负数 400。
func TestZeroBidOverHTTP(t *testing.T) {
	r := newTestApp(t)
	_, out := doJSON(t, r, "POST", "/api/sessions", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	_, out = doJSON(t, r, "POST", "/api/departments", `{"name":"零价组","expected_count":1}`, token)
	dept := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/roles", `{"name":"zeroBidder","description":"","permissions":["admissions.record"]}`, token)
	role := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/users", `{"username":"zeroA","name":"零价出价人","password":"pass","role_ids":[`+itoa(role)+`],"department_id":`+itoa(dept)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/sessions", `{"username":"zeroA","password":"pass"}`, "")
	bidder, _ := out["token"].(string)
	if bidder == "" {
		t.Fatalf("零价出价人登录失败: %v", out)
	}

	_, out = doJSON(t, r, "POST", "/api/candidates", `{"student_no":"2024099","name":"零价候选人"}`, token)
	cand := int(out["id"].(float64))
	if code, _ := doJSON(t, r, "PATCH", "/api/system/status", `{"phase":"leftover"}`, token); code != http.StatusOK {
		t.Fatal("切到捡漏阶段失败")
	}

	if code, _ := doJSON(t, r, "PUT", "/api/leftover/bids/"+itoa(cand), `{"amount":-1}`, bidder); code != http.StatusBadRequest {
		t.Fatalf("负数出价应 400: %d", code)
	}
	if code, got := doJSON(t, r, "PUT", "/api/leftover/bids/"+itoa(cand), `{"amount":0}`, bidder); code != http.StatusOK {
		t.Fatalf("0 出价应 200: %d %v", code, got)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/leftover/bids/"+itoa(cand), `{"amount":0}`, bidder); code != http.StatusOK {
		t.Fatalf("重复 0 出价应 200（更新既有记录）: %d", code)
	}
	amounts := func() []float64 {
		t.Helper()
		code, out := doJSON(t, r, "GET", "/api/leftover/bids", "", bidder)
		if code != http.StatusOK {
			t.Fatalf("bids: %d", code)
		}
		items, _ := out["items"].([]any)
		got := make([]float64, 0, len(items))
		for _, it := range items {
			got = append(got, it.(map[string]any)["amount"].(float64))
		}
		return got
	}
	if got := amounts(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("出价列表应为 [0]，实得 %v", got)
	}
	if code, _ := doJSON(t, r, "PUT", "/api/leftover/bids/"+itoa(cand), `{"amount":300}`, bidder); code != http.StatusOK {
		t.Fatalf("0 改价应 200: %d", code)
	}
	if got := amounts(); len(got) != 1 || got[0] != 300 {
		t.Fatalf("改价后应为 [300]（单条记录），实得 %v", got)
	}
}
