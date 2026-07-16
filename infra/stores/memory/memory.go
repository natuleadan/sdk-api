package memory

import (
	"sync"
	"time"
)

type Item struct {
	Value  any
	Expiry int64
}

type Memory struct {
	mu       sync.Mutex
	items    map[string]*Item
	gcDone   chan struct{}
	gcTick   time.Duration
}

func New(cleanupInterval time.Duration) *Memory {
	if cleanupInterval <= 0 {
		cleanupInterval = 5 * time.Minute
	}
	m := &Memory{
		items:  make(map[string]*Item),
		gcDone: make(chan struct{}),
		gcTick: cleanupInterval,
	}
	go m.gcLoop()
	return m
}

func (m *Memory) Get(key string) any {
	m.mu.Lock()
	defer m.mu.Unlock()

	it, ok := m.items[key]
	if !ok {
		return nil
	}
	if it.Expiry > 0 && time.Now().UnixNano() >= it.Expiry {
		delete(m.items, key)
		return nil
	}
	return it.Value
}

func (m *Memory) Set(key string, value any, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var expiry int64
	if ttl > 0 {
		expiry = time.Now().Add(ttl).UnixNano()
	}
	m.items[key] = &Item{Value: value, Expiry: expiry}
}

func (m *Memory) Delete(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.items, key)
}

func (m *Memory) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

func (m *Memory) Close() {
	close(m.gcDone)
}

func (m *Memory) gcLoop() {
	ticker := time.NewTicker(m.gcTick)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.gc()
		case <-m.gcDone:
			return
		}
	}
}

func (m *Memory) gc() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UnixNano()
	for k, v := range m.items {
		if v.Expiry > 0 && now >= v.Expiry {
			delete(m.items, k)
		}
	}
}
