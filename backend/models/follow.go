package models

import (
	"time"

	"gorm.io/gorm"
)

// Follow 关注关系模型
type Follow struct {
	ID         uint           `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint           `gorm:"index:idx_user_follow,unique;not null" json:"user_id"`     // 关注者
	FollowedID uint           `gorm:"index:idx_user_follow,unique;not null" json:"followed_id"` // 被关注者
	CreatedAt  time.Time      `json:"created_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

func (Follow) TableName() string {
	return "follows"
}
