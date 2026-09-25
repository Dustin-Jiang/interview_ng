package handler

import "encoding/json"

// Envelope 是 WS 消息的统一 JSON 信封（Q10=A）。
// {"op":..., "req_id":..., "data":...}
type Envelope struct {
	Op    string          `json:"op"`
	ReqID string          `json:"req_id,omitempty"`
	Type  string          `json:"type,omitempty"` // 服务端主动推送时用 type 区分
	Data  json.RawMessage `json:"data"`
}

//---- 客户端 -> 服务端 命令 ----

// reqAuth 连接后首条鉴权消息：携带 JWT，成功后绑定 userID 并按路径 roomId 自动 JoinRoom。
type reqAuth struct {
	Token string `json:"token"`
}

type reqSync struct {
	LastMsgID uint64 `json:"last_msg_id"`
}

type reqSendMsg struct {
	Content   string  `json:"content"`
	ReplyToID *uint64 `json:"reply_to_id"`
}

type reqMovePhase struct {
	To string `json:"to"`
}

//---- 服务端推送 ----

// chanEvent 推送给客户端的事件（内含 EventData/Message 快照）。
type chanEvent struct {
	Type   string          `json:"type"`
	RoomID uint64          `json:"room_id"`
	Seq    uint64          `json:"seq"`
	MsgID  uint64          `json:"msg_id,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}
