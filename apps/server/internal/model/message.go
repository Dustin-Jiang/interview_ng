package model

import "time"

// Message 面试群聊记录（面试档案），按候选人归属。
// (candidate_id, id) 唯一，id 即候选人维度续传游标 —— 断线重连时客户端带 lastMsgID 续传。
// 消息属于候选人而非房间：候选人换房/解绑后，历史消息随人走。
// SenderID 可空（OnDelete:SET NULL）：面试官被删后其消息保留（候选人档案不受销毁），sender 置空。
type Message struct {
	ID          uint64  `gorm:"primaryKey" json:"id"`
	CandidateID uint64  `gorm:"index:idx_candidate_id" json:"candidate_id"`
	SenderID    *uint64 `gorm:"index" json:"sender_id"`
	Sender      *User   `gorm:"foreignKey:SenderID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;" json:"sender,omitempty"`
	Content     string  `gorm:"type:text" json:"content"`
	// ReplyToID 本消息引用的先行消息（同候选人内，可空）。只存 id，不存内容快照：
	// 被引用的消息撤回（物理删除）后该 id 悬空，读取方在自己已加载的记录里按 id 现查，
	// 查不到就显示「引用的消息已撤回」——引用不保留内容副本，撤回的语义就是内容消失。
	ReplyToID *uint64   `gorm:"index" json:"reply_to_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	// Reactions 表情回复：读取时预加载（`ListMessagesAfter` 统一带上）。
	// 只暴露「谁 + 哪个表情」——计数与「我回没回」由前端按当前用户聚合（事件因此与观察者无关）。
	Reactions []MessageReaction `gorm:"foreignKey:MessageID" json:"reactions"`
}

// MessageReaction 一条记录上的一个表情回复：同一 (message_id, user_id, emoji) 唯一，
// 即「一个人对同一条记录的同一个表情」只能有一份（重复提交走幂等，不新增行）。
// 撤回消息 / 候选人被删（连带删消息）时随之消失——由 store 显式删除（不依赖 FK 级联的具体行为），
// 约束声明仍保留为兜底。
// User 一并序列化（不含凭据字段）：右键菜单要显示「谁回了这个表情」，前端不必再查用户表。
type MessageReaction struct {
	ID        uint64    `gorm:"primaryKey" json:"-"`
	MessageID uint64    `gorm:"uniqueIndex:idx_reaction_person_emoji" json:"-"`
	UserID    uint64    `gorm:"uniqueIndex:idx_reaction_person_emoji" json:"user_id"`
	Emoji     string    `gorm:"size:16;uniqueIndex:idx_reaction_person_emoji" json:"emoji"`
	CreatedAt time.Time `json:"-"`
	Message   *Message  `gorm:"foreignKey:MessageID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	User      *User     `gorm:"foreignKey:UserID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"user,omitempty"`
}

// ReactionEmojis 允许的表情回复集合（唯一权威）：按「态度 → 评价 → 关注 → 其他」分组排列，
// 顺序即前端选择器里的展示顺序（`domain/messages.ts#REACTION_EMOJIS` 是同一口径的镜像，
// 仅决定候选按钮；服务端永远复核）。面试记录是正式档案，故限定为一小把语义明确的常用表情，
// 不引入任意字符输入（列宽 16 字符，全部表情均在 8 字节内）。
var ReactionEmojis = []string{
	// 态度
	"👍", "👎", "❤️", "🎉", "😂", "😕",
	// 评价
	"✅", "❌", "💯", "⭐", "👏", "🔥",
	// 关注
	"🤔", "👀", "😮", "🙏", "🤝", "✨",
	// 其他
	"🤯", "😴", "🚀", "⚡", "⚠️", "😭",
}

// ValidReaction 判断表情是否在允许集内。
func ValidReaction(emoji string) bool {
	for _, e := range ReactionEmojis {
		if e == emoji {
			return true
		}
	}
	return false
}
