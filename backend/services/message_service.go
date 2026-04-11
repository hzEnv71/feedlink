package services

import (
	"errors"
	"feed/models"
	"feed/realtime"
	"feed/repository"
	"strings"
	"time"
)

// MessageService 负责私信会话与消息读写流程。
type MessageService struct {
	messageRepo repository.MessageRepository
	userRepo    repository.UserRepository
}

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
	return &MessageService{
		messageRepo: repository.NewMessageRepository(models.DB),
		userRepo:    repository.NewUserRepository(models.DB),
	}
}

// SendMessage 发送私信：校验收发双方与内容后写入消息表。
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

	if _, err := s.userRepo.GetByID(req.ToUserID); err != nil {
		return nil, errors.New("接收方用户不存在")
	}

	msg := &models.Message{
		FromUserID: fromUserID,
		ToUserID:   req.ToUserID,
		Content:    content,
		CreatedAt:  time.Now(),
	}
	if err := s.messageRepo.Create(msg); err != nil {
		return nil, errors.New("发送失败")
	}

	// 实时推送：对方在线时立即收到新消息事件。
	realtime.PushToUser(req.ToUserID, realtime.MessageEvent{ //接收方：读取接收者接受的消息（写入消息 前端读取）
		Type: "message:new",
		Data: msg,
	})

	return msg, nil
}

// GetConversationMessages 获取会话消息，并将“对方->当前用户”的未读消息标记为已读。
func (s *MessageService) GetConversationMessages(currentUserID, targetUserID uint, page, pageSize int) ([]models.Message, int64, error) {
	if currentUserID == 0 || targetUserID == 0 {
		return nil, 0, errors.New("参数错误")
	}
	list, total, err := s.messageRepo.GetByConversation(currentUserID, targetUserID, page, pageSize)
	if err != nil {
		return nil, 0, err
	}
	_ = s.messageRepo.MarkReadFromTo(targetUserID, currentUserID)
	return list, total, nil
}

func (s *MessageService) GetConversationList(currentUserID uint, page, pageSize int) ([]ConversationItem, int64, error) {
	if currentUserID == 0 {
		return nil, 0, errors.New("请先登录")
	}

	messages, err := s.messageRepo.ListByUser(currentUserID)
	if err != nil {
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

		unread, _ := s.messageRepo.CountUnreadFromTo(targetID, currentUserID)
		conversationMap[targetID] = ConversationItem{
			User:       targetUser.ToResponse(),
			LastMsg:    msg.Content,
			LastTime:   msg.CreatedAt.Format(time.RFC3339),
			Unread:     unread,
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
