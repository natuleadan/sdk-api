package memory

import (
	"testing"
	"time"
)

func TestSetGet(t *testing.T) {
	m := New(0)
	defer m.Close()

	m.Set("key1", "value1", 0)
	if v := m.Get("key1"); v != "value1" {
		t.Errorf("expected value1, got %v", v)
	}
}

func TestGetMissing(t *testing.T) {
	m := New(0)
	defer m.Close()

	if v := m.Get("noexist"); v != nil {
		t.Errorf("expected nil, got %v", v)
	}
}

func TestDelete(t *testing.T) {
	m := New(0)
	defer m.Close()

	m.Set("key1", "value1", 0)
	m.Delete("key1")
	if v := m.Get("key1"); v != nil {
		t.Errorf("expected nil after delete, got %v", v)
	}
}

func TestTTLExpiry(t *testing.T) {
	m := New(100 * time.Millisecond)
	defer m.Close()

	m.Set("key1", "value1", 50*time.Millisecond)
	if v := m.Get("key1"); v != "value1" {
		t.Errorf("expected value1 before expiry, got %v", v)
	}

	time.Sleep(100 * time.Millisecond)
	if v := m.Get("key1"); v != nil {
		t.Errorf("expected nil after TTL, got %v", v)
	}
}

func TestTTLNoExpiry(t *testing.T) {
	m := New(100 * time.Millisecond)
	defer m.Close()

	m.Set("key1", "value1", 0)
	time.Sleep(200 * time.Millisecond)
	if v := m.Get("key1"); v != "value1" {
		t.Errorf("expected value1 (no TTL = no expiry), got %v", v)
	}
}

func TestGCEvictsExpired(t *testing.T) {
	m := New(50 * time.Millisecond)
	defer m.Close()

	m.Set("key1", "value1", 30*time.Millisecond)
	m.Set("key2", "value2", 0) // no expiry

	time.Sleep(100 * time.Millisecond)

	if v := m.Get("key1"); v != nil {
		t.Error("expected key1 to be GC'd")
	}
	if v := m.Get("key2"); v != "value2" {
		t.Error("expected key2 to survive (no TTL)")
	}
}

func TestLen(t *testing.T) {
	m := New(0)
	defer m.Close()

	m.Set("a", 1, 0)
	m.Set("b", 2, 0)
	if n := m.Len(); n != 2 {
		t.Errorf("expected len 2, got %d", n)
	}
}

func TestConcurrentSetGet(t *testing.T) {
	m := New(0)
	defer m.Close()

	done := make(chan struct{})
	go func() {
		for i := range 100 {
			m.Set("key", i, 0)
		}
		close(done)
	}()

	for range 100 {
		m.Get("key")
	}
	<-done
}
