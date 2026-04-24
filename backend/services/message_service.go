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
	if fromUserID == 0 { //如果发送者ID为0，则返回请先登录事件
		return nil, errors.New("请先登录")
	}
	if req.ToUserID == 0 || req.ToUserID == fromUserID { //如果接收者ID为0或等于发送者ID，则返回接收方无效事件
		return nil, errors.New("接收方无效")
	}

	content := strings.TrimSpace(req.Content) //去除消息内容前后空格
	if content == "" {
		return nil, errors.New("消息内容不能为空") //发送消息内容不能为空事件
	}

	if _, err := s.userRepo.GetByID(req.ToUserID); err != nil { //获取接收者用户
		return nil, errors.New("接收方用户不存在") //发送接收者用户不存在事件
	}

	msg := &models.Message{ //创建消息
		FromUserID: fromUserID,
		ToUserID:   req.ToUserID, //设置接收者ID
		Content:    content,      //设置消息内容
		CreatedAt:  time.Now(),   //设置创建时间
	}
	if err := s.messageRepo.Create(msg); err != nil { //创建消息
		return nil, errors.New("发送失败") //发送发送失败事件
	}

	// 实时推送：对方在线时立即收到新消息事件。
	realtime.PushToUser(req.ToUserID, realtime.MessageEvent{ //接收方：读取接收者接受的消息（写入消息 前端读取）
		Type: "message:new", //设置消息类型
		Data: msg, //设置消息数据
	})

	return msg, nil //返回消息
}

// GetConversationMessages 获取会话消息，并将“对方->当前用户”的未读消息标记为已读。
func (s *MessageService) GetConversationMessages(currentUserID, targetUserID uint, page, pageSize int) ([]models.Message, int64, error) {
	if currentUserID == 0 || targetUserID == 0 { //如果当前用户ID或目标用户ID为0，则返回参数错误事件
		return nil, 0, errors.New("参数错误")
	}
	list, total, err := s.messageRepo.GetByConversation(currentUserID, targetUserID, page, pageSize) //获取会话消息
	if err != nil {
		return nil, 0, err //发送获取会话消息失败事件
	}
	_ = s.messageRepo.MarkReadFromTo(targetUserID, currentUserID) //标记已读
	return list, total, nil                                       //返回会话消息
}

// 获取会话列表
func (s *MessageService) GetConversationList(currentUserID uint, page, pageSize int) ([]ConversationItem, int64, error) {
	if currentUserID == 0 {
		return nil, 0, errors.New("请先登录") //发送请先登录事件
	}

	messages, err := s.messageRepo.ListByUser(currentUserID)
	if err != nil {
		return nil, 0, err //发送获取会话列表失败事件
	}

	conversationMap := map[uint]ConversationItem{} //创建会话映射
	for _, msg := range messages {
		targetID := msg.ToUserID             //获取目标用户ID
		targetUser := msg.ToUser             //获取目标用户
		if msg.FromUserID != currentUserID { //如果发送者ID不等于当前用户ID
			targetID = msg.FromUserID //获取发送者ID
			targetUser = msg.FromUser //获取发送者
		}

		if _, exists := conversationMap[targetID]; exists { //如果会话映射中存在目标用户ID，则跳过
			continue
		}

		unread, _ := s.messageRepo.CountUnreadFromTo(targetID, currentUserID) //获取未读消息数
		conversationMap[targetID] = ConversationItem{
			User:       targetUser.ToResponse(),            //转换为目标用户响应
			LastMsg:    msg.Content,                        //获取最后一条消息内容
			LastTime:   msg.CreatedAt.Format(time.RFC3339), //获取最后一条消息时间
			Unread:     unread,                             //获取未读消息数
			TargetID:   targetID,                           //获取目标用户ID
			TargetName: targetUser.Nickname,                //获取目标用户昵称
		}
	}

	all := make([]ConversationItem, 0, len(conversationMap)) //创建会话列表
	for _, item := range conversationMap {
		all = append(all, item) //添加会话列表
	}

	total := int64(len(all))       //获取会话列表总数
	start := (page - 1) * pageSize //获取起始位置
	if start >= len(all) {
		return []ConversationItem{}, total, nil //发送会话列表为空事件
	}
	end := min(start+pageSize, len(all)) //获取结束位置

	return all[start:end], total, nil //返回会话列表
}
