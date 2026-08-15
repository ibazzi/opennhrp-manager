package service

import (
	"sync"

	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/protocol"
)

type LogHub struct {
	clients map[*websocket.Conn]bool
	mu      sync.RWMutex
}

func NewLogHub() *LogHub {
	return &LogHub{
		clients: make(map[*websocket.Conn]bool),
	}
}

func (h *LogHub) Register(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[conn] = true
}

func (h *LogHub) Unregister(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, conn)
}

func (h *LogHub) Broadcast(entry protocol.LogEntry) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.clients {
		_ = conn.WriteJSON(entry)
	}
}
