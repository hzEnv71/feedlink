package handlers

import (
	"feed/middleware"
	"feed/services"
	"feed/utils"
	"strconv"

	"github.com/gin-gonic/gin"
)

type MessageHandler struct {
	messageService *services.MessageService
}

func NewMessageHandler() *MessageHandler {
	return &MessageHandler{messageService: services.NewMessageService()}
}

// SendMessage 发送私信
// POST /api/messages
func (h *MessageHandler) SendMessage(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)

	var req services.SendMessageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.Error(c, 400, "参数错误: "+err.Error())
		return
	}

	msg, err := h.messageService.SendMessage(currentUserID, &req)
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessWithMessage(c, "发送成功", msg)
}

// GetConversations 获取会话列表
// GET /api/messages/conversations
func (h *MessageHandler) GetConversations(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 20
	}

	list, total, err := h.messageService.GetConversationList(currentUserID, page, pageSize)
	if err != nil {
		utils.Error(c, 500, "获取会话失败")
		return
	}

	utils.SuccessPage(c, list, total, page, pageSize)
}

// GetConversationMessages 获取会话消息
// GET /api/messages/:target_id
func (h *MessageHandler) GetConversationMessages(c *gin.Context) {
	currentUserID := middleware.GetCurrentUserID(c)
	targetIDStr := c.Param("target_id")
	targetID, err := strconv.ParseUint(targetIDStr, 10, 64)
	if err != nil {
		utils.Error(c, 400, "用户ID无效")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "30"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}

	list, total, err := h.messageService.GetConversationMessages(currentUserID, uint(targetID), page, pageSize)
	if err != nil {
		utils.Error(c, 400, err.Error())
		return
	}

	utils.SuccessPage(c, list, total, page, pageSize)
}
