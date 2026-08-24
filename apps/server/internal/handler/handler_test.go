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
		&dsmodel.Department{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	cache := rbac.New()
	if err := seed.Init(context.Background(), db, cache); err != nil {
		t.Fatalf("seed: %v", err)
	}
	am := auth.New("test-secret", 7*24*time.Hour, db, cache)
	store := state.NewMemStateStore(db)
	b := broadcast.New()
	svc := service.New(store, b)

	r := gin.New()
	handler.NewHTTPServer(svc, store, am).RegisterRoutes(r)
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
	code, _ := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"wrong"}`, "")
	if code != http.StatusUnauthorized {
		t.Fatalf("bad login: got %d", code)
	}

	// 正确登录 → 200 + token
	code, out := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
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

	_, out := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
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

	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"reader1","password":"pass"}`, "")
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

	_, out := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 准备：候选人签到 → 建房 → 拉取 → 加入（让房间有候选人与成员）
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"name":"张三","profile":"后端"}`, token)
	candID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/candidates/"+itoa(candID)+"/checkin", "", token)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/pull_candidate", `{"candidate_id":`+itoa(candID)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	adminID := int(out["user"].(map[string]any)["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/members", `{"user_id":`+itoa(adminID)+`}`, token)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/room/" + itoa(roomID)
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

	_, out := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 准备：候选人 A/B 签到；建房 → 拉取 A → 加入成员
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"name":"甲","profile":"后端"}`, token)
	candA := int(out["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"name":"乙","profile":"前端"}`, token)
	candB := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/candidates/"+itoa(candA)+"/checkin", "", token)
	doJSON(t, r, "POST", "/api/candidates/"+itoa(candB)+"/checkin", "", token)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/pull_candidate", `{"candidate_id":`+itoa(candA)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	adminID := int(out["user"].(map[string]any)["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/members", `{"user_id":`+itoa(adminID)+`}`, token)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/room/" + itoa(roomID)
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
	code, pullOut := doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/pull_candidate", `{"candidate_id":`+itoa(candB)+`}`, token)
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

	_, out := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
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
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"itvA","password":"pass"}`, "")
	tokenA := out["token"].(string)
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"itvB","password":"pass"}`, "")
	tokenB := out["token"].(string)

	srv := httptest.NewServer(r)
	defer srv.Close()

	dial := func(room int) *websocket.Conn {
		t.Helper()
		url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/room/" + itoa(room)
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
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"name":"新签","profile":"前端"}`, token)
	candID := int(out["id"].(float64))
	if code, _ := doJSON(t, r, "POST", "/api/candidates/"+itoa(candID)+"/checkin", "", token); code != http.StatusOK {
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

	_, out := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 准备：候选人签到 → 建房 → 拉取 → 加入成员
	_, out = doJSON(t, r, "POST", "/api/candidates", `{"name":"甲","profile":"后端"}`, token)
	candID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/candidates/"+itoa(candID)+"/checkin", "", token)
	_, out = doJSON(t, r, "POST", "/api/rooms", "", token)
	roomID := int(out["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/pull_candidate", `{"candidate_id":`+itoa(candID)+`}`, token)
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	adminID := int(out["user"].(map[string]any)["id"].(float64))
	doJSON(t, r, "POST", "/api/rooms/"+itoa(roomID)+"/members", `{"user_id":`+itoa(adminID)+`}`, token)

	srv := httptest.NewServer(r)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws/room/" + itoa(roomID)
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
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"viewer1","password":"pass"}`, "")
	vToken := out["token"].(string)
	if code, _ := doJSON(t, r, "GET", "/api/candidates/"+itoa(candID)+"/messages", "", vToken); code != http.StatusOK {
		t.Fatalf("viewer transcript: got %d", code)
	}

	// 无任何权限的普通用户也可查看归档（查看与管理分离）
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"nobody","name":"","password":"pass","role_ids":[]}`, token)
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"nobody","password":"pass"}`, "")
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

	_, out := doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	token := out["token"].(string)

	// 默认部门已由种子创建，admin 归属其中
	_, out = doJSON(t, r, "GET", "/api/me", "", token)
	if user, _ := out["user"].(map[string]any); user["department"] == nil {
		t.Fatalf("admin should have a department: %v", out)
	}

	// 创建部门
	code, out := doJSON(t, r, "POST", "/api/departments", `{"name":"前端组","description":"负责前端岗位"}`, token)
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
		}
	}
	if !found {
		t.Fatalf("created department not listed: %v", out)
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

	// 无 users.manage 权限用户（interviewer 角色）访问部门接口 → 403
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"admin","password":"admin"}`, "")
	code, itvRole := doJSON(t, r, "POST", "/api/roles", `{"name":"itv2","description":"","permissions":["rooms.view"]}`, token)
	if code != http.StatusCreated {
		t.Fatalf("create role: got %d %v", code, itvRole)
	}
	roleID := int(itvRole["id"].(float64))
	_, out = doJSON(t, r, "POST", "/api/users", `{"username":"itv2u","name":"","password":"pass","role_ids":[`+itoa(roleID)+`]}`, token)
	_, out = doJSON(t, r, "POST", "/api/auth/login", `{"username":"itv2u","password":"pass"}`, "")
	tokenB := out["token"].(string)
	if code, _ := doJSON(t, r, "GET", "/api/departments", "", tokenB); code != http.StatusForbidden {
		t.Fatalf("interviewer list departments: got %d", code)
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
