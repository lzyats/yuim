package groupcache

import (
	"sync"
	"time"
)

type entry struct {
	uids []int64
	exp  time.Time
}

type Cache struct {
	mu  sync.RWMutex
	ttl time.Duration
	m   map[int64]entry
}

func New(ttl time.Duration) *Cache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Cache{ttl: ttl, m: make(map[int64]entry)}
}

func (c *Cache) Get(groupID int64) ([]int64, bool) {
	c.mu.RLock()
	e, ok := c.m[groupID]
	c.mu.RUnlock()
	if !ok || time.Now().After(e.exp) {
		return nil, false
	}
	return e.uids, true
}

func (c *Cache) Set(groupID int64, uids []int64) {
	c.mu.Lock()
	c.m[groupID] = entry{uids: uids, exp: time.Now().Add(c.ttl)}
	c.mu.Unlock()
}
