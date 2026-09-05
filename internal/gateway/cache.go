package gateway

import (
	"container/list"
	"sync"
	"time"
)

type cacheEntry struct {
	key     string
	vectors [][]float32
	bytes   int
	until   time.Time
}

// Cache is a byte/entry/TTL-bounded LRU. Callers supply a complete authority/space/input key.
// Values and slices are always copied; expiry and eviction never broaden a cache lookup.
type Cache struct {
	mu                          sync.Mutex
	entries                     map[string]*list.Element
	lru                         *list.List
	maxEntries, maxBytes, bytes int
	ttl                         time.Duration
	now                         func() time.Time
}

func NewCache(entries, bytes int, ttl time.Duration, now func() time.Time) *Cache {
	if now == nil {
		now = time.Now
	}
	return &Cache{entries: map[string]*list.Element{}, lru: list.New(), maxEntries: entries, maxBytes: bytes, ttl: ttl, now: now}
}
func CloneVectors(in [][]float32) [][]float32 {
	out := make([][]float32, len(in))
	for i, v := range in {
		out[i] = append([]float32(nil), v...)
	}
	return out
}
func (c *Cache) Get(key string) ([][]float32, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	v := e.Value.(cacheEntry)
	if !c.now().Before(v.until) {
		c.remove(e)
		return nil, false
	}
	c.lru.MoveToFront(e)
	return CloneVectors(v.vectors), true
}
func (c *Cache) Put(key string, vectors [][]float32) {
	size := len(key) + len(vectors)*24
	for _, v := range vectors {
		size += len(v) * 4
	}
	if c.maxEntries <= 0 || size > c.maxBytes || c.ttl <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if e := c.entries[key]; e != nil {
		c.remove(e)
	}
	for c.lru.Len() >= c.maxEntries || c.bytes+size > c.maxBytes {
		c.remove(c.lru.Back())
	}
	c.entries[key] = c.lru.PushFront(cacheEntry{key, CloneVectors(vectors), size, c.now().Add(c.ttl)})
	c.bytes += size
}
func (c *Cache) remove(e *list.Element) {
	if e == nil {
		return
	}
	v := e.Value.(cacheEntry)
	delete(c.entries, v.key)
	c.bytes -= v.bytes
	c.lru.Remove(e)
}
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*list.Element{}
	c.lru.Init()
	c.bytes = 0
}
