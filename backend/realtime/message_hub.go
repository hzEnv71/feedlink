package realtime

import (
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

// MessageEvent 为消息实时推送事件。
type MessageEvent struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

var (
	hubMu       sync.RWMutex
	userSockets = map[uint]map[*websocket.Conn]struct{}{}
)

// RegisterConn 注册用户 WebSocket 连接。
func RegisterConn(userID uint, conn *websocket.Conn) {
	hubMu.Lock()
	defer hubMu.Unlock()
	if _, ok := userSockets[userID]; !ok {
		userSockets[userID] = map[*websocket.Conn]struct{}{}
	}
	userSockets[userID][conn] = struct{}{}
}

// UnregisterConn 注销用户 WebSocket 连接。
func UnregisterConn(userID uint, conn *websocket.Conn) {
	hubMu.Lock()
	defer hubMu.Unlock()
	if conns, ok := userSockets[userID]; ok {
		delete(conns, conn)
		if len(conns) == 0 {
			delete(userSockets, userID)
		}
	}
}

// PushToUser 推送事件到用户所有在线连接。
func PushToUser(userID uint, event MessageEvent) {
	hubMu.RLock()
	conns := userSockets[userID]
	hubMu.RUnlock()
	if len(conns) == 0 {
		return
	}

	payload, _ := json.Marshal(event)
	for conn := range conns {
		_ = conn.WriteMessage(websocket.TextMessage, payload)
	}
}
