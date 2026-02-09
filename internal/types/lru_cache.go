package types

import (
	"container/list"
	"sync"
)

// LRUCache 带 LRU 淘汰策略的缓存，防止内存无限增长
type LRUCache struct {
	capacity int
	mu       sync.Mutex
	cache    map[string]*list.Element
	lru      *list.List
}

type cacheEntry struct {
	key   string
	value *FileInfo
}

// NewLRUCache 创建容量限制的 LRU 缓存
func NewLRUCache(capacity int) *LRUCache {
	if capacity <= 0 {
		capacity = 1000
	}
	return &LRUCache{
		capacity: capacity,
		cache:    make(map[string]*list.Element, capacity),
		lru:      list.New(),
	}
}

// Load 获取缓存值，若存在则移至最近使用
func (c *LRUCache) Load(key string) (*FileInfo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		return elem.Value.(*cacheEntry).value, true
	}
	return nil, false
}

// Store 存入缓存，若超出容量则淘汰最久未使用的项
func (c *LRUCache) Store(key string, value *FileInfo) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).value = value
		return
	}

	// 容量满时淘汰尾部
	if c.lru.Len() >= c.capacity {
		oldest := c.lru.Back()
		if oldest != nil {
			c.lru.Remove(oldest)
			delete(c.cache, oldest.Value.(*cacheEntry).key)
		}
	}

	elem := c.lru.PushFront(&cacheEntry{key: key, value: value})
	c.cache[key] = elem
}
