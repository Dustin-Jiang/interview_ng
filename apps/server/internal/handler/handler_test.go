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
