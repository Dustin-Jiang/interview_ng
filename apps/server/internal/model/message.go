package model

import "time"

// Message 面试群聊记录（面试档案），按候选人归属。
// (candidate_id, id) 唯一，id 即候选人维度续传游标 —— 断线重连时客户端带 lastMsgID 续传。
// 消息属于候选人而非房间：候选人换房/解绑后，历史消息随人走。
// SenderID 可空（OnDelete:SET NULL）：面试官被删后其消息保留（候选人档案不受销毁），sender 置空。
type Message struct {
	ID          uint64    `gorm:"primaryKey" json:"id"`
	CandidateID uint64    `gorm:"index:idx_candidate_id" json:"candidate_id"`
	SenderID    *uint64   `gorm:"index" json:"sender_id"`
	Sender      *User     `gorm:"foreignKey:SenderID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"sender,omitempty"`
	Content     string    `gorm:"type:text" json:"content"`
	CreatedAt   time.Time `json:"created_at"`
}
