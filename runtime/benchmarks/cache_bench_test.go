package benchmarks

import (
	"sync"
	"testing"
)

type testCache struct {
	mu    sync.RWMutex
	items map[string]string
}

func BenchmarkCacheGet(b *testing.B) {
	c := &testCache{items: map[string]string{"key": "value"}}
	b.ReportAllocs()
	for b.Loop() {
		c.mu.RLock()
		_ = c.items["key"]
		c.mu.RUnlock()
	}
}

func BenchmarkCacheSet(b *testing.B) {
	c := &testCache{items: make(map[string]string)}
	b.ReportAllocs()
	for b.Loop() {
		c.mu.Lock()
		c.items["key"] = "value"
		c.mu.Unlock()
	}
}
