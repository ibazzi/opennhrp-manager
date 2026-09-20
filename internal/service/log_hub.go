package service

import (
	"sync"

	"github.com/gorilla/websocket"

	"opennhrp-manager/internal/protocol"
)

type LogHub struct {
	clients     map[*websocket.Conn]bool
	subscribers map[chan protocol.LogEntry]struct{}
	mu          sync.RWMutex
}

func NewLogHub() *LogHub {
	return &LogHub{
		clients:     make(map[*websocket.Conn]bool),
		subscribers: make(map[chan protocol.LogEntry]struct{}),
	}
}

func (h *LogHub) Subscribe() (<-chan protocol.LogEntry, func()) {
	entries := make(chan protocol.LogEntry, 32)
	h.mu.Lock()
	h.subscribers[entries] = struct{}{}
	h.mu.Unlock()
	return entries, func() {
		h.mu.Lock()
		delete(h.subscribers, entries)
		close(entries)
		h.mu.Unlock()
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
	h.mu.Lock()
	defer h.mu.Unlock()

	for conn := range h.clients {
		_ = conn.WriteJSON(entry)
	}
	for entries := range h.subscribers {
		select {
		case entries <- entry:
		default:
			// ponytail: live UI logs are lossy under bursts; persist them if complete history becomes required.
		}
	}
}
