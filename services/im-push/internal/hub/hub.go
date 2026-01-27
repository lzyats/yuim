package hub

import (
	"sync"

	"github.com/gorilla/websocket"
)

type Conn struct {
	UID int64
	WS  *websocket.Conn
	// bounded outbound queue (backpressure)
	Out chan []byte
}

type Hub struct {
	mu    sync.RWMutex
	conns map[int64]*Conn
}

func New() *Hub {
	return &Hub{conns: make(map[int64]*Conn)}
}

func (h *Hub) Set(uid int64, c *Conn) {
	h.mu.Lock()
	h.conns[uid] = c
	h.mu.Unlock()
}

func (h *Hub) Get(uid int64) (*Conn, bool) {
	h.mu.RLock()
	c, ok := h.conns[uid]
	h.mu.RUnlock()
	return c, ok
}

func (h *Hub) Del(uid int64) {
	h.mu.Lock()
	delete(h.conns, uid)
	h.mu.Unlock()
}

func (h *Hub) Len() int {
	h.mu.RLock()
	n := len(h.conns)
	h.mu.RUnlock()
	return n
}
