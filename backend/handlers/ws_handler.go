package handlers

import (
	"encoding/json"
	"feed/middleware"
	"feed/realtime"
	"feed/services"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WSHandler 负责 WebSocket 连接升级、统一鉴权与消息协议处理。
type WSHandler struct {
	messageService *services.MessageService
}

func NewWSHandler(messageService *services.MessageService) *WSHandler {
	return &WSHandler{messageService: messageService}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

const (
	wsReadTimeout     = 70 * time.Second
	wsPongWait        = 70 * time.Second
	wsPingInterval    = 25 * time.Second
	wsWriteWait       = 10 * time.Second
	wsSendRatePerSec  = 5.0
	wsSendBurst       = 10
	maxMessageContent = 1000
)

type wsInboundEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type wsSendMessageData struct {
	ToUserID    uint   `json:"to_user_id"`
	Content     string `json:"content"`
	ClientMsgID string `json:"client_msg_id"`
}

type wsConversationsData struct {
	ReqID    string `json:"req_id"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
}

type wsHistoryData struct {
	ReqID      string `json:"req_id"`
	TargetUser uint   `json:"target_user_id"`
	Page       int    `json:"page"`
	PageSize   int    `json:"page_size"`
}

type wsTokenBucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newWSTokenBucket(rate float64, burst int) *wsTokenBucket {
	b := float64(burst)
	return &wsTokenBucket{rate: rate, burst: b, tokens: b, last: time.Now()}
}

func (tb *wsTokenBucket) Allow() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(tb.last).Seconds()
	if elapsed > 0 {
		tb.tokens = math.Min(tb.burst, tb.tokens+elapsed*tb.rate)
		tb.last = now
	}
	if tb.tokens < 1 {
		return false
	}
	tb.tokens -= 1
	return true
}

// MessageWS 建立私信实时通道。
// GET /ws/messages?token=xxx
func (h *WSHandler) MessageWS(c *gin.Context) {
	claims, err := middleware.ParseTokenFromRequest(c)
	if err != nil || claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid token"})
		return
	}

	userID := middleware.GetCurrentUserID(c)
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "unauthorized"})
		return
	}
	if claims.ExpiresAt == nil || claims.ExpiresAt.Time.Before(time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "token expired"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	client := &realtime.Client{
		UserID:    userID,
		Conn:      conn,
		ExpiresAt: claims.ExpiresAt.Time,
	}
	realtime.RegisterConn(client)
	defer func() {
		realtime.UnregisterConn(client)
		_ = conn.Close()
	}()

	limiter := newWSTokenBucket(wsSendRatePerSec, wsSendBurst)
	stopPing := make(chan struct{})
	go h.keepAlive(client, stopPing)

	for {
		if time.Now().After(client.ExpiresAt) {
			_ = client.WriteJSON(realtime.MessageEvent{Type: "auth:expired", Data: gin.H{"message": "token expired"}})
			_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "token expired"), time.Now().Add(2*time.Second))
			break
		}

		_, payload, err := conn.ReadMessage() //发送方：读取发送者发送的消息（读取前端发送的消息）
		if err != nil {
			break
		}

		if !limiter.Allow() {
			_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"message": "发送过于频繁，请稍后再试"}})
			continue
		}

		var event wsInboundEvent
		if err := json.Unmarshal(payload, &event); err != nil {
			_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"message": "invalid payload"}})
			continue
		}
		// log.Println("ReadMessage", string(payload))

		switch strings.TrimSpace(event.Type) {
		case "message:send":
			h.handleMessageSend(client, event.Data)
		case "message:conversations":
			h.handleConversations(client, event.Data)
		case "message:history":
			h.handleHistory(client, event.Data)
		default:
			_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"message": "unsupported event type"}})
		}
	}

	close(stopPing)
}

func (h *WSHandler) handleMessageSend(client *realtime.Client, raw json.RawMessage) {
	var req wsSendMessageData
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"message": "invalid message payload"}})
		return
	}

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" || len([]rune(req.Content)) > maxMessageContent {
		_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"client_msg_id": req.ClientMsgID, "message": "消息内容长度无效"}})
		return
	}

	msg, err := h.messageService.SendMessage(client.UserID, &services.SendMessageRequest{
		ToUserID: req.ToUserID,
		Content:  req.Content,
	})
	if err != nil {
		_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"client_msg_id": req.ClientMsgID, "message": err.Error()}})
		return
	}

	_ = client.WriteJSON(realtime.MessageEvent{Type: "message:ack", Data: gin.H{"client_msg_id": req.ClientMsgID, "message": msg}})
}

func (h *WSHandler) handleConversations(client *realtime.Client, raw json.RawMessage) {
	var req wsConversationsData
	_ = json.Unmarshal(raw, &req)
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 50 {
		req.PageSize = 50
	}

	list, total, err := h.messageService.GetConversationList(client.UserID, req.Page, req.PageSize)
	if err != nil {
		_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"req_id": req.ReqID, "message": "获取会话失败"}})
		return
	}

	_ = client.WriteJSON(realtime.MessageEvent{Type: "message:conversations", Data: gin.H{"req_id": req.ReqID, "list": list, "total": total, "page": req.Page, "page_size": req.PageSize}})
}

func (h *WSHandler) handleHistory(client *realtime.Client, raw json.RawMessage) {
	var req wsHistoryData
	if err := json.Unmarshal(raw, &req); err != nil {
		_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"message": "invalid history payload"}})
		return
	}
	if req.TargetUser == 0 {
		_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"req_id": req.ReqID, "message": "target_user_id 无效"}})
		return
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 100
	}

	list, total, err := h.messageService.GetConversationMessages(client.UserID, req.TargetUser, req.Page, req.PageSize)
	if err != nil {
		_ = client.WriteJSON(realtime.MessageEvent{Type: "message:error", Data: gin.H{"req_id": req.ReqID, "message": err.Error()}})
		return
	}

	_ = client.WriteJSON(realtime.MessageEvent{Type: "message:history", Data: gin.H{"req_id": req.ReqID, "target_user_id": req.TargetUser, "list": list, "total": total, "page": req.Page, "page_size": req.PageSize}})
}

func (h *WSHandler) keepAlive(client *realtime.Client, stop <-chan struct{}) {
	ticker := time.NewTicker(wsPingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			if time.Now().After(client.ExpiresAt) {
				_ = client.WriteJSON(realtime.MessageEvent{Type: "auth:expired", Data: gin.H{"message": "token expired"}})
				_ = client.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "token expired"), time.Now().Add(2*time.Second))
				_ = client.Conn.Close()
				return
			}
			_ = client.Conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := client.WriteControl(websocket.PingMessage, []byte("ping"), time.Now().Add(wsWriteWait)); err != nil {
				_ = client.Conn.Close()
				return
			}
		}
	}
}
