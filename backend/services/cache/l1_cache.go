package cache

import (
	"container/list"
	"sync"
	"time"
)

// l1Item stores the cached value and metadata in the LRU list.
type l1Item struct {
	key       string
	value     []byte
	sizeBytes int64
	expiresAt time.Time
}

func (item *l1Item) expired() bool {
	return !item.expiresAt.IsZero() && time.Now().After(item.expiresAt)
}

// L1Cache is a thread-safe, size-bounded LRU in-memory cache.
type L1Cache struct {
	mu           sync.RWMutex
	items        map[string]*list.Element
	evictList    *list.List
	maxEntries   int
	maxSizeBytes int64
	currentBytes int64
	stats        MemoryStats
}

// NewL1Cache initializes a new size-bounded LRU L1 cache.
func NewL1Cache(maxEntries int, maxSizeBytes int64) *L1Cache {
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	if maxSizeBytes <= 0 {
		maxSizeBytes = 64 * 1024 * 1024 // 64 MB default
	}

	c := &L1Cache{
		items:        make(map[string]*list.Element),
		evictList:    list.New(),
		maxEntries:   maxEntries,
		maxSizeBytes: maxSizeBytes,
	}
	c.stats.MaxEntries = maxEntries
	c.stats.MaxSizeBytes = maxSizeBytes
	return c
}

// Get retrieves a key from the L1 cache, updating LRU recency.
func (c *L1Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, found := c.items[key]
	if !found {
		c.stats.TotalMisses.Add(1)
		return nil, false
	}

	item := elem.Value.(*l1Item)
	if item.expired() {
		c.removeElement(elem)
		c.stats.TotalEvictions.Add(1)
		c.stats.TotalMisses.Add(1)
		return nil, false
	}

	c.evictList.MoveToFront(elem)
	c.stats.TotalHits.Add(1)

	// Return a defensive copy to prevent external mutation
	res := make([]byte, len(item.value))
	copy(res, item.value)
	return res, true
}

// Set stores a key-value pair in L1 cache, evicting LRU items if limits are exceeded.
func (c *L1Cache) Set(key string, value []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	itemSize := EstimateEntrySize(key, value)

	// If entry alone exceeds max capacity, do not cache in L1
	if itemSize > c.maxSizeBytes {
		return
	}

	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = time.Now().Add(ttl)
	}

	// If already present, update value and move to front
	if elem, exists := c.items[key]; exists {
		oldItem := elem.Value.(*l1Item)
		c.currentBytes -= oldItem.sizeBytes
		c.stats.UsedSizeBytes.Add(-oldItem.sizeBytes)

		oldItem.value = value
		oldItem.sizeBytes = itemSize
		oldItem.expiresAt = expiresAt
		c.currentBytes += itemSize
		c.stats.UsedSizeBytes.Add(itemSize)

		c.evictList.MoveToFront(elem)
		c.evictIfNeeded()
		return
	}

	// Evict items if adding this will exceed entry or size bounds
	for (len(c.items) >= c.maxEntries || (c.currentBytes+itemSize > c.maxSizeBytes)) && c.evictList.Len() > 0 {
		c.evictOldest()
	}

	newItem := &l1Item{
		key:       key,
		value:     value,
		sizeBytes: itemSize,
		expiresAt: expiresAt,
	}

	elem := c.evictList.PushFront(newItem)
	c.items[key] = elem
	c.currentBytes += itemSize

	c.stats.TotalEntries.Store(int64(len(c.items)))
	c.stats.UsedSizeBytes.Store(c.currentBytes)
}

// Delete removes a key from the L1 cache.
func (c *L1Cache) Delete(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, exists := c.items[key]; exists {
		c.removeElement(elem)
		return true
	}
	return false
}

// Clear wipes all entries from the L1 cache.
func (c *L1Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element)
	c.evictList.Init()
	c.currentBytes = 0
	c.stats.TotalEntries.Store(0)
	c.stats.UsedSizeBytes.Store(0)
}

// Len returns the current count of items stored in L1 cache.
func (c *L1Cache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.items)
}

// Stats returns memory and usage statistics.
func (c *L1Cache) Stats() *MemoryStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return &c.stats
}

func (c *L1Cache) evictIfNeeded() {
	for (len(c.items) > c.maxEntries || c.currentBytes > c.maxSizeBytes) && c.evictList.Len() > 0 {
		c.evictOldest()
	}
}

func (c *L1Cache) evictOldest() {
	oldest := c.evictList.Back()
	if oldest != nil {
		c.removeElement(oldest)
		c.stats.TotalEvictions.Add(1)
	}
}

func (c *L1Cache) removeElement(elem *list.Element) {
	c.evictList.Remove(elem)
	item := elem.Value.(*l1Item)
	delete(c.items, item.key)
	c.currentBytes -= item.sizeBytes

	c.stats.TotalEntries.Store(int64(len(c.items)))
	c.stats.UsedSizeBytes.Store(c.currentBytes)
}
