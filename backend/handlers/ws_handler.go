package handlers

import (
	"feed/middleware"
	"feed/realtime"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WSHandler 负责 WebSocket 连接升级与注册。
type WSHandler struct{}

func NewWSHandler() *WSHandler { return &WSHandler{} }

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// MessageWS 建立私信实时通道。
// GET /ws/messages?token=xxx
func (h *WSHandler) MessageWS(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "token required"})
		return
	}

	claims, err := middleware.ParseTokenFromQuery(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "invalid token"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	userID := claims.UserID
	realtime.RegisterConn(userID, conn)
	defer realtime.UnregisterConn(userID, conn)

	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}
