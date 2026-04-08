package services

import (
	"errors"
	"feed/models"
	"strings"
	"time"
)

type MessageService struct{}

type SendMessageRequest struct {
	ToUserID uint   `json:"to_user_id" binding:"required"`
	Content  string `json:"content" binding:"required,min=1,max=1000"`
}

type ConversationItem struct {
	User       models.UserResponse `json:"user"`
	LastMsg    string              `json:"last_msg"`
	LastTime   string              `json:"last_time"`
	Unread     int64               `json:"unread"`
	TargetID   uint                `json:"target_id"`
	TargetName string              `json:"target_name"`
}

func NewMessageService() *MessageService {
	return &MessageService{}
}

func (s *MessageService) SendMessage(fromUserID uint, req *SendMessageRequest) (*models.Message, error) {
	if fromUserID == 0 {
		return nil, errors.New("请先登录")
	}
	if req.ToUserID == 0 || req.ToUserID == fromUserID {
		return nil, errors.New("接收方无效")
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, errors.New("消息内容不能为空")
	}

	var target models.User
	if err := models.DB.Where("id = ?", req.ToUserID).First(&target).Error; err != nil {
		return nil, errors.New("接收方用户不存在")
	}

	msg := &models.Message{
		FromUserID: fromUserID,
		ToUserID:   req.ToUserID,
		Content:    content,
		CreatedAt:  time.Now(),
	}
	if err := models.DB.Create(msg).Error; err != nil {
		return nil, errors.New("发送失败")
	}

	return msg, nil
}

func (s *MessageService) GetConversationMessages(currentUserID, targetUserID uint, page, pageSize int) ([]models.Message, int64, error) {
	if currentUserID == 0 || targetUserID == 0 {
		return nil, 0, errors.New("参数错误")
	}

	var messages []models.Message
	var total int64

	query := models.DB.Model(&models.Message{}).
		Where("(from_user_id = ? AND to_user_id = ?) OR (from_user_id = ? AND to_user_id = ?)",
			currentUserID, targetUserID, targetUserID, currentUserID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Preload("FromUser").Preload("ToUser").Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&messages).Error; err != nil {
		return nil, 0, err
	}

	return messages, total, nil
}

func (s *MessageService) GetConversationList(currentUserID uint, page, pageSize int) ([]ConversationItem, int64, error) {
	if currentUserID == 0 {
		return nil, 0, errors.New("请先登录")
	}

	var messages []models.Message
	if err := models.DB.Where("from_user_id = ? OR to_user_id = ?", currentUserID, currentUserID).
		Order("created_at DESC").
		Preload("FromUser").
		Preload("ToUser").
		Find(&messages).Error; err != nil {
		return nil, 0, err
	}

	conversationMap := map[uint]ConversationItem{}
	for _, msg := range messages {
		targetID := msg.ToUserID
		targetUser := msg.ToUser
		if msg.FromUserID != currentUserID {
			targetID = msg.FromUserID
			targetUser = msg.FromUser
		}

		if _, exists := conversationMap[targetID]; exists {
			continue
		}

		conversationMap[targetID] = ConversationItem{
			User:       targetUser.ToResponse(),
			LastMsg:    msg.Content,
			LastTime:   msg.CreatedAt.Format(time.RFC3339),
			Unread:     0,
			TargetID:   targetID,
			TargetName: targetUser.Nickname,
		}
	}

	all := make([]ConversationItem, 0, len(conversationMap))
	for _, item := range conversationMap {
		all = append(all, item)
	}

	total := int64(len(all))
	start := (page - 1) * pageSize
	if start >= len(all) {
		return []ConversationItem{}, total, nil
	}
	end := min(start+pageSize, len(all))

	return all[start:end], total, nil
}
