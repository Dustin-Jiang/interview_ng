package model

// RoomMember 房间成员 —— 一个面试官在一个房间的席位。
// user_id 唯一约束保证"同一位面试官一次只在 ≤1 个活跃房间"(Q7=A)。
type RoomMember struct {
	ID     uint64 `gorm:"primaryKey" json:"id"`
	RoomID uint64 `gorm:"uniqueIndex:idx_room_user" json:"room_id"`
	UserID uint64 `gorm:"uniqueIndex:idx_room_user" json:"user_id"`
	User   *User  `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"user,omitempty"`
}
