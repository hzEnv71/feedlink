package services

import (
	"feed/models"
	"feed/repository"
	"time"
)

// NotificationService 负责通知中心业务编排。
type NotificationService struct {
	notificationRepo repository.NotificationRepository
}

type NotificationItem struct {
	ID        uint                `json:"id"`
	Type      string              `json:"type"`
	TargetID  uint                `json:"target_id"`
	Content   string              `json:"content"`
	IsRead    bool                `json:"is_read"`
	CreatedAt string              `json:"created_at"`
	Actor     models.UserResponse `json:"actor"`
}

func NewNotificationService() *NotificationService {
	return &NotificationService{notificationRepo: repository.NewNotificationRepository(models.DB)}
}

// CreateLikeNotification 创建点赞通知。
func (s *NotificationService) CreateLikeNotification(actorID, receiverID, feedID uint) {
	if actorID == 0 || receiverID == 0 || actorID == receiverID {
		return
	}
	_ = s.notificationRepo.Create(&models.Notification{
		UserID:   receiverID,
		ActorID:  actorID,
		Type:     models.NotificationTypeLike,
		TargetID: feedID,
		Content:  "赞了你的动态",
	})
}

// CreateCommentNotification 创建评论通知。
func (s *NotificationService) CreateCommentNotification(actorID, receiverID, feedID uint, content string) {
	if actorID == 0 || receiverID == 0 || actorID == receiverID {
		return
	}
	_ = s.notificationRepo.Create(&models.Notification{
		UserID:   receiverID,
		ActorID:  actorID,
		Type:     models.NotificationTypeComment,
		TargetID: feedID,
		Content:  "评论了你的动态: " + content,
	})
}

// CreateFollowNotification 创建关注通知。
func (s *NotificationService) CreateFollowNotification(actorID, receiverID uint) {
	if actorID == 0 || receiverID == 0 || actorID == receiverID {
		return
	}
	_ = s.notificationRepo.Create(&models.Notification{
		UserID:  receiverID,
		ActorID: actorID,
		Type:    models.NotificationTypeFollow,
		Content: "关注了你",
	})
}

// ListNotifications 获取通知列表及未读数。
func (s *NotificationService) ListNotifications(userID uint, page, pageSize int) ([]NotificationItem, int64, int64, error) {
	list, total, err := s.notificationRepo.ListByUser(userID, page, pageSize)
	if err != nil {
		return nil, 0, 0, err
	}
	unread, _ := s.notificationRepo.CountUnread(userID)

	result := make([]NotificationItem, 0, len(list))
	for _, n := range list {
		result = append(result, NotificationItem{
			ID:        n.ID,
			Type:      n.Type,
			TargetID:  n.TargetID,
			Content:   n.Content,
			IsRead:    n.IsRead,
			CreatedAt: n.CreatedAt.Format(time.RFC3339),
			Actor:     n.Actor.ToResponse(),
		})
	}
	return result, total, unread, nil
}

// MarkAllRead 全部标记已读。
func (s *NotificationService) MarkAllRead(userID uint) error {
	return s.notificationRepo.MarkAllRead(userID)
}
