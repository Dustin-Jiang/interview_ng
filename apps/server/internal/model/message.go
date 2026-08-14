package model

import "time"

// Message 房间内的群聊交流（面试记录），长期落库。
// (room_id, id) 唯一，id 即房间内续传游标 —— 断线重连时客户端带 lastMsgID 续传。
type Message struct {
	ID        uint64    `gorm:"primaryKey" json:"id"`
	RoomID    uint64    `gorm:"index:idx_room_id" json:"room_id"`
	SenderID  uint64    `json:"sender_id"`
	Sender    *User     `gorm:"foreignKey:SenderID;constraint:OnUpdate:CASCADE,OnDelete:RESTRICT;" json:"sender,omitempty"`
	Content   string    `gorm:"type:text" json:"content"`
	CreatedAt time.Time `json:"created_at"`
}
