package models

import "time"

// Message 私信实体。
// 说明：
// - 会话由 (from_user_id, to_user_id) 双向聚合得到；
// - created_at 用于聊天记录与会话列表排序。
type Message struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	FromUserID uint      `gorm:"index:idx_conversation_time;not null" json:"from_user_id"`
	ToUserID   uint      `gorm:"index:idx_conversation_time;not null" json:"to_user_id"`
	Content    string    `gorm:"type:varchar(1000);not null" json:"content"`
	IsRead     bool      `gorm:"index;default:false" json:"is_read"`
	CreatedAt  time.Time `gorm:"index:idx_conversation_time" json:"created_at"`

	FromUser User `gorm:"foreignKey:FromUserID" json:"from_user"`
	ToUser   User `gorm:"foreignKey:ToUserID" json:"to_user"`
}

func (Message) TableName() string {
	return "messages"
}
