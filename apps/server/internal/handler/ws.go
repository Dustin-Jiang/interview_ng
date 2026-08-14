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

	"interview_ng/internal/broadcast"
	"interview_ng/internal/model"
	"interview_ng/internal/service"
	"interview_ng/internal/state"
)

const (
	writeWait  = 10 * time.Second
	pongWait   = 60 * time.Second
	pingPeriod = (pongWait * 9) / 10
	maxMsgSize = 4096
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
	userID uint64
	roomID uint64

	send chan []byte // 带缓冲的写队列，由 writer goroutine 消费；广播与命令回复都走这里

	mu        sync.Mutex // 串行化对底层 conn 的并发写保护
	lastSeq   uint64     // 该连接所见最近事件 Seq（幂等对齐）
	lastMsgID uint64     // 该连接所见最近消息 id（续传游标）
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
	svc *service.InterviewService
	b   *broadcast.Manager
	st  state.StateStore
	h   *hub
}

// NewWSServer 构建 WS 服务器。
func NewWSServer(svc *service.InterviewService, b *broadcast.Manager, st state.StateStore) *WSServer {
	return &WSServer{svc: svc, b: b, st: st, h: newHub()}
}

// RegisterRoutes 注册 WS 与相关路由。
func (w *WSServer) RegisterRoutes(r *gin.Engine) {
	r.GET("/ws/room", w.serveWS)
}

// serveWS 处理 WS 升级与连接生命周期。
func (w *WSServer) serveWS(c *gin.Context) {
	userID, _ := strconv.ParseUint(c.Query("user_id"), 10, 64)
	roomID, _ := strconv.ParseUint(c.Query("room_id"), 10, 64)
	if userID == 0 || roomID == 0 {
		c.JSON(400, gin.H{"error": "user_id and room_id required"})
		return
	}
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &wsClient{conn: conn, userID: userID, roomID: roomID, send: make(chan []byte, 256)}
	w.h.add(roomID, client)

	// 入房（若尚未是成员则 JoinRoom 持久化 + 广播）。
	room, _, err := w.st.JoinRoom(c.Request.Context(), roomID, userID)
	if err != nil {
		client.writeBatch(mustRawEnvelope("sync", map[string]any{"error": err.Error()}))
		conn.Close()
		w.h.remove(roomID, client)
		return
	}
	_ = room

	// 建立扇出：该房间首个连接时注册 sink，把事件推送到该房间所有连接。
	w.ensureSink(roomID)

	// readPump 返回(连接断开)时通过 quit 通知 writePump 退出并关闭连接。
	quit := make(chan struct{})
	go w.writePump(client, quit)

	go w.readPump(c.Request.Context(), client, quit)

	<-quit
	w.h.remove(roomID, client)
}

// ensureSink 为房间注册扇出回调；已注册则忽略（幂等）。
// sink 负责把事件推给该房间内的所有当前连接的写队列。
func (w *WSServer) ensureSink(roomID uint64) {
	ok := w.b.Bind(roomID, func(ev *state.Event) {
		frame := encodeEvent(ev)
		if frame == nil {
			return
		}
		for _, cl := range w.h.clientsIn(roomID) {
			cl.enqueue(frame)
		}
	})
	_ = ok
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
	for {
		_, message, err := client.conn.ReadMessage()
		if err != nil {
			break
		}
		var env Envelope
		if err := json.Unmarshal(message, &env); err != nil {
			continue
		}
		w.dispatch(ctx, client, &env)
	}
	_ = client.conn.Close()
	closeSafe(quit)
}

// dispatch 根据 op 分发到具体命令处理。
func (w *WSServer) dispatch(ctx context.Context, client *wsClient, env *Envelope) {
	switch env.Op {
	case "sync":
		var req reqSync
		_ = json.Unmarshal(env.Data, &req)
		w.handleSync(ctx, client, &req, env.ReqID)
	case "send_msg":
		var req reqSendMsg
		_ = json.Unmarshal(env.Data, &req)
		w.handleSendMsg(ctx, client, &req, env.ReqID)
	case "join":
		var req reqJoin
		_ = json.Unmarshal(env.Data, &req)
		w.handleJoin(ctx, client, &req, env.ReqID)
	case "move_phase":
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

// handleSync 处理断线/首次同步：返回房间快照 + 消息增量。
func (w *WSServer) handleSync(ctx context.Context, client *wsClient, req *reqSync, reqID string) {
	room, err := w.st.GetRoom(ctx, client.roomID)
	if err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	msgs, err := w.st.ListMessagesAfter(ctx, client.roomID, req.LastMsgID)
	if err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
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
	if err := w.svc.SendMessage(ctx, req.RoomID, client.userID, req.Content); err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// 广播由 service 的 Publish 触发，此处只回执。
	w.reply(client, reqID, map[string]any{"ok": true, "op": "send_msg", "room_id": req.RoomID})
}

// handleJoin 处理面试官加入房间。
func (w *WSServer) handleJoin(ctx context.Context, client *wsClient, req *reqJoin, reqID string) {
	room, _, err := w.st.JoinRoom(ctx, req.RoomID, client.userID)
	if err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	msgs, _ := w.st.ListMessagesAfter(ctx, req.RoomID, 0)
	w.reply(client, reqID, map[string]any{"ok": true, "room": room, "messages": msgs})
}

// handleMovePhase 推进阶段。
func (w *WSServer) handleMovePhase(ctx context.Context, client *wsClient, req *reqMovePhase, reqID string) {
	to := model.CandidateStatus(req.To)
	if err := w.svc.MovePhase(ctx, req.RoomID, client.userID, to); err != nil {
		w.reply(client, reqID, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	w.reply(client, reqID, map[string]any{"ok": true, "room_id": req.RoomID, "phase": req.To})
}

//--- helpers ---

func mustRawEnvelope(intype string, data any) []byte {
	b, err := json.Marshal(&Envelope{Type: intype, Data: mustRaw(data)})
	if err != nil {
		return nil
	}
	return b
}

func mustRaw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
