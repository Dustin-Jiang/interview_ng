package model

// RoomMember 房间成员 —— 一个面试官在一个房间的席位。
// 联合唯一索引 idx_room_user 只保证「同一个人在同一间房只有一条席位」；
// 面试官同时可以是多间房的成员（不再有「一次只在一个活跃房间」的限制）。
type RoomMember struct {
	ID     uint64 `gorm:"primaryKey" json:"id"`
	RoomID uint64 `gorm:"uniqueIndex:idx_room_user" json:"room_id"`
	UserID uint64 `gorm:"uniqueIndex:idx_room_user" json:"user_id"`
	User   *User  `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"user,omitempty"`
}
