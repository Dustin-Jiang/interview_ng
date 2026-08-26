package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"interview_ng/internal/auth"
	"interview_ng/internal/broadcast"
	dsmodel "interview_ng/internal/model"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

const (
	writeWait   = 10 * time.Second
	pongWait    = 60 * time.Second
	pingPeriod  = (pongWait * 9) / 10
	maxMsgSize  = 4096
	authTimeout = 10 * time.Second // 连接后 10s 内必须完成 auth，否则断开（Q13=A）

	// boardHubKey hub 内保留键：看板通道客户端集合（非真实房间）。
	// 房间路径的 roomId 必须大于 0，键 0 不会与任何真实房间冲突。
	boardHubKey = 0
)

// closeSafe 幂等地关闭一个 channel。
func closeSafe(ch chan struct{}) {
	defer func() { _ = recover() }() // 重复 close 会 panic，此处吞掉
	close(ch)
}

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(*http.Request) bool { return true },
}

// wsClient 代表一条已鉴权的 WS 连接及其会话状态。
type wsClient struct {
	conn   *websocket.Conn
	userID uint64 // 经 auth 消息绑定，之后不再信任客户端
	roomID uint64

	send chan []byte // 带缓冲的写队列，由 writer goroutine 消费；广播与命令回复都走这里

	mu        sync.Mutex // 串行化对底层 conn 的并发写保护
	lastMsgID uint64     // 该连接所见最近消息 id（续传游标，按候选人维度）
}

func (c *wsClient) enqueue(frame []byte) {
	select {
	case c.send <- frame:
	default:
		// 缓冲满：慢消费者，丢弃并交由重连续传补齐。
	}
}

func (c *wsClient) writeBatch(frames ...[]byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	for _, f := range frames {
		if err := c.conn.WriteMessage(websocket.TextMessage, f); err != nil {
			return
		}
	}
}

// WSServer 承载 WS 连接生命周期、命令分发与房间扇出。
type WSServer struct {
	svc  *service.InterviewService
	b    *broadcast.Manager
	st   state.StateStore
	h    *hub
	auth *auth.Manager

	boardSinkOnce sync.Once // 全局事件扇出回调仅注册一次
}

// NewWSServer 构建 WS 服务器。
func NewWSServer(svc *service.InterviewService, b *broadcast.Manager, st state.StateStore, am *auth.Manager) *WSServer {
	return &WSServer{svc: svc, b: b, st: st, h: newHub(), auth: am}
}

// RegisterRoutes 注册 WS 与相关路由。
func (w *WSServer) RegisterRoutes(r *gin.Engine) {
	r.GET("/ws/room/:roomId", w.serveWS)
	r.GET("/ws/board", w.serveBoard)
}

// serveWS 处理 WS 升级与连接生命周期。
// 握手不带任何业务参数（RESTful 路径承载 roomId，Q12=B）；身份经连接后首条 auth 消息绑定。
func (w *WSServer) serveWS(c *gin.Context) {
	roomID, _ := strconv.ParseUint(c.Param("roomId"), 10, 64)
	if roomID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid roomId"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &wsClient{conn: conn, roomID: roomID, send: make(chan []byte, 256)}
	quit := make(chan struct{})

	go w.writePump(client, quit)
	go w.readPump(c.Request.Context(), client, quit)

	<-quit
	// 连接断开即离开房间（释放"一次一个活跃房间"席位）；
	// 断线重连/刷新由客户端重新 auth → JoinRoom 幂等重入。
	if client.userID != 0 {
		w.svc.RemoveRoomMember(c.Request.Context(), roomID, client.userID)
	}
	w.h.remove(roomID, client)
}

// authTimer 为连接启动鉴权超时：10s 内未完成 auth 则断开。
func (w *WSServer) authTimer(client *wsClient, quit chan struct{}) *time.Timer {
	return time.AfterFunc(authTimeout, func() {
		// 未鉴权 → 直接断开
		if client.userID == 0 {
			_ = client.conn.Close()
			closeSafe(quit)
		}
	})
}

// ensureSink 为房间注册扇出回调；已注册则忽略（幂等）。
// sink 负责把事件推给该房间内的所有当前连接的写队列。
func (w *WSServer) ensureSink(roomID uint64) {
	w.b.Bind(roomID, func(ev *state.Event) {
		frame := encodeEvent(ev)
		if frame == nil {
			return
		}
		if ev.RoomID == 0 {
			// 全局事件（签到/拉走等"待分配池"变化）：推给所有开放房间的连接。
			// 看板客户端（boardHubKey）由 BindGlobal 扇出，此处排除以免重复投递。
			for _, cl := range w.h.clientsAllExcept(boardHubKey) {
				cl.enqueue(frame)
			}
			return
		}
		for _, cl := range w.h.clientsIn(roomID) {
			cl.enqueue(frame)
		}
	})
}

// ensureBoardSink 注册全局事件扇出回调（幂等）：所有事件（房间级 + 全局级）
// 推给所有已鉴权的看板连接，供列表页（候场大屏 / 房间列表 / 候选人记录）触发刷新。
func (w *WSServer) ensureBoardSink() {
	w.boardSinkOnce.Do(func() {
		w.b.BindGlobal(func(ev *state.Event) {
			frame := encodeEvent(ev)
			if frame == nil {
				return
			}
			for _, cl := range w.h.clientsIn(boardHubKey) {
				cl.enqueue(frame)
			}
		})
	})
}

// serveBoard 处理看板通道的 WS 升级与连接生命周期。
// 与房间通道的区别：不 JoinRoom、无成员语义、只接受 auth 一条命令；
// 鉴权要求 rooms.view（事件载荷可能含消息内容，与 REST 读记录的权限对齐）。
func (w *WSServer) serveBoard(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &wsClient{conn: conn, send: make(chan []byte, 256)}
	quit := make(chan struct{})

	go w.writePump(client, quit)
	go w.readBoardPump(c.Request.Context(), client, quit)

	<-quit
	if client.userID != 0 {
		w.h.remove(boardHubKey, client)
	}
}

// readBoardPump 看板通道读循环：鉴权前仅接受 auth；成功后进入纯接收模式。
func (w *WSServer) readBoardPump(ctx context.Context, client *wsClient, quit chan struct{}) {
	client.conn.SetReadLimit(maxMsgSize)
	_ = client.conn.SetReadDeadline(time.Now().Add(pongWait))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	timer := w.authTimer(client, quit)
	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			break
		}
		var env Envelope
		if err := json.Unmarshal(message, &env); err != nil {
			continue
		}
		if env.Op != "auth" {
			w.reply(client, env.ReqID, map[string]any{"ok": false, "error": "unknown op"})
			continue
		}
		var req reqAuth
		_ = json.Unmarshal(env.Data, &req)
		if !w.handleBoardAuth(client, &req, env.ReqID) {
			break // 鉴权失败：由 authTimer/关闭收尾
		}
	}
	timer.Stop()
	_ = client.conn.Close()
	closeSafe(quit)
}

// replySync 同步写一条回执（绕过写队列）。
// 用于鉴权失败等"回执后立即断开"的场景：入队可能来不及被 writePump 消费。
func (w *WSServer) replySync(client *wsClient, reqID string, data any) {
	frame, err := json.Marshal(&Envelope{Type: "reply", ReqID: reqID, Data: mustRaw(data)})
	if err != nil {
		return
	}
	client.writeBatch(frame)
}

// handleBoardAuth 校验 JWT + rooms.view 权限，成功即加入看板客户端集合并注册全局扇出。
// 返回 false 表示鉴权失败（调用方应结束读循环、断开连接）。
func (w *WSServer) handleBoardAuth(client *wsClient, req *reqAuth, reqID string) bool {
	if client.authed() {
		w.reply(client, reqID, map[string]any{"ok": false, "error": "重复鉴权"})
		return true
	}
	uid, err := w.auth.Authenticate(req.Token)
	if err != nil {
		w.replySync(client, reqID, map[string]any{"ok": false, "error": "鉴权失败"})
		return false
	}
	if !w.auth.HasPermission(uid, dsmodel.PermRoomsView) {
		w.replySync(client, reqID, map[string]any{"ok": false, "error": "无查看权限"})
		return false
	}
	client.userID = uid
	w.h.add(boardHubKey, client)
	w.ensureBoardSink()
	w.reply(client, reqID, map[string]any{"ok": true})
	return true
}

// encodeEvent 将 state 事件序列化为客户端可读的 chanEvent。
func encodeEvent(ev *state.Event) []byte {
	data, _ := json.Marshal(ev.Data)
	out := chanEvent{Type: string(ev.Type), RoomID: ev.RoomID, Seq: ev.Seq, MsgID: ev.MsgID, Data: data}
	b, err := json.Marshal(&Envelope{Type: string(ev.Type), Data: mustRaw(out)})
	if err != nil {
		return nil
	}
	return b
}

// writePump 消费写队列把帧写到网络连接，并发送心跳。
// 当 quit 关闭(读端断开)或写失败时退出。
func (w *WSServer) writePump(client *wsClient, quit chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()
	for {
		select {
		case frame, ok := <-client.send:
			if !ok {
				closeSafe(quit)
				return
			}
			client.writeBatch(frame)
		case <-ticker.C:
			_ = client.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := client.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				closeSafe(quit)
				return
			}
		case <-quit:
			return
		}
	}
}

// readPump 读取客户端命令并分发；出错时关闭 quit 并关闭连接。
func (w *WSServer) readPump(ctx context.Context, client *wsClient, quit chan struct{}) {
	client.conn.SetReadLimit(maxMsgSize)
	_ = client.conn.SetReadDeadline(time.Now().Add(pongWait))
	client.conn.SetPongHandler(func(string) error {
		return client.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	timer := w.authTimer(client, quit)
	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			break
		}
		var env Envelope
		if err := json.Unmarshal(message, &env); err != nil {
			continue
		}
		// 鉴权成功前：仅允许 auth op，其余一律拒绝。
		if !client.authed() && env.Op != "auth" {
			w.reply(client, env.ReqID, map[string]any{"ok": false, "error": "尚未鉴权"})
			continue
		}
		w.dispatch(ctx, client, &env)
	}
	timer.Stop()
	_ = client.conn.Close()
	closeSafe(quit)
}

func (c *wsClient) authed() bool { return c.userID != 0 }

// dispatch 根据 op 分发到具体命令处理。
func (w *WSServer) dispatch(ctx context.Context, client *wsClient, env *Envelope) {
	switch env.Op {
	case "auth":
		var req reqAuth
		_ = json.Unmarshal(env.Data, &req)
		w.handleAuth(ctx, client, &req, env.ReqID)
	case "sync":
		var req reqSync
		_ = json.Unmarshal(env.Data, &req)
		w.handleSync(ctx, client, &req, env.ReqID)
	case "send_msg":
		if !w.auth.HasPermission(client.userID, dsmodel.PermRoomsChat) {
			w.reply(client, env.ReqID, map[string]any{"ok": false, "error": "无聊天权限"})
			return
		}
		var req reqSendMsg
		_ = json.Unmarshal(env.Data, &req)
		w.handleSendMsg(ctx, client, &req, env.ReqID)
	case "move_phase":
		if !w.auth.HasPermission(client.userID, dsmodel.PermRoomsMovePhase) {
			w.reply(client, env.ReqID, map[string]any{"ok": false, "error": "无阶段推进权限"})
			return
		}
		var req reqMovePhase
		_ = json.Unmarshal(env.Data, &req)
		w.handleMovePhase(ctx, client, &req, env.ReqID)
	default:
		w.reply(client, env.ReqID, map[string]any{"ok": false, "error": "unknown op"})
	}
}

func (w *WSServer) reply(client *wsClient, reqID string, data any) {
	frame, err := json.Marshal(&Envelope{Type: "reply", ReqID: reqID, Data: mustRaw(data)})
	if err != nil {
		return
	}
	client.enqueue(frame)
}

// handleAuth 处理首条鉴权消息：校验 JWT + token_version + rooms.chat 权限，
// 成功即绑定 userID 并按路径 roomID 自动 JoinRoom。
func (w *WSServer) handleAuth(ctx context.Context, client *wsClient, req *reqAuth, reqID string) {
	if client.authed() {
		w.reply(client, reqID, map[string]any{"ok": false, "error": "重复鉴权"})
		return
	}
	uid, err := w.auth.Authenticate(req.Token)
	if err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": "鉴权失败"})
		return
	}
	if !w.auth.HasPermission(uid, dsmodel.PermRoomsChat) {
		w.reply(client, reqID, map[string]any{"ok": false, "error": "无进房权限"})
		return
	}
	// 自动 JoinRoom（先落库后广播）：成功后才绑定身份并入 hub，失败则保持未鉴权（超时断开）。
	if err := w.svc.AddRoomMember(ctx, client.roomID, uid); err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	client.userID = uid
	w.h.add(client.roomID, client)
	w.ensureSink(client.roomID)
	w.reply(client, reqID, map[string]any{"ok": true, "room_id": client.roomID})
}

// handleSync 处理断线/首次同步：返回房间快照 + 消息增量（按候选人维度续传）。
func (w *WSServer) handleSync(ctx context.Context, client *wsClient, req *reqSync, reqID string) {
	room, err := w.st.GetRoom(ctx, client.roomID)
	if err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	var msgs []*dsmodel.Message
	if room.CandidateID != nil {
		msgs, err = w.st.ListMessagesAfter(ctx, *room.CandidateID, req.LastMsgID)
		if err != nil {
			w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
			return
		}
	}
	client.lastMsgID = req.LastMsgID
	for _, m := range msgs {
		if m.ID > client.lastMsgID {
			client.lastMsgID = m.ID
		}
	}
	w.reply(client, reqID, map[string]any{"ok": true, "room": room, "messages": msgs})
}

// handleSendMsg 发送一条聊天消息（service 先落库后广播）。
func (w *WSServer) handleSendMsg(ctx context.Context, client *wsClient, req *reqSendMsg, reqID string) {
	if err := w.svc.SendMessage(ctx, client.roomID, client.userID, req.Content); err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// 广播由 service 的 Publish 触发，此处只回执。
	w.reply(client, reqID, map[string]any{"ok": true, "op": "send_msg", "room_id": client.roomID})
}

// handleMovePhase 推进阶段（权限在 dispatch 层已校验）。
func (w *WSServer) handleMovePhase(ctx context.Context, client *wsClient, req *reqMovePhase, reqID string) {
	to := dsmodel.CandidateStatus(req.To)
	if err := w.svc.MovePhase(ctx, client.roomID, client.userID, to); err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	w.reply(client, reqID, map[string]any{"ok": true, "room_id": client.roomID, "phase": req.To})
}

//--- helpers ---

func mustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
