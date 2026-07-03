package openfga

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

var errNotFound = errors.New("not found")

type memBackend struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func (m *memBackend) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, errNotFound
	}
	return v, nil
}

func (m *memBackend) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.data == nil {
		m.data = make(map[string][]byte)
	}
	m.data[key] = value
	return nil
}

func (m *memBackend) Close() error { return nil }

type mockClient struct {
	mu      sync.Mutex
	checkFn func(ctx context.Context, req CheckRequest) (bool, error)
	callLog []CheckRequest
}

func (m *mockClient) Check(ctx context.Context, req CheckRequest) (bool, error) {
	m.mu.Lock()
	m.callLog = append(m.callLog, req)
	m.mu.Unlock()
	if m.checkFn != nil {
		return m.checkFn(ctx, req)
	}
	return true, nil
}

func TestNewCachedClient_ReturnsNonNil(t *testing.T) {
	cc := NewCachedClient(nil, nil)
	if cc == nil {
		t.Fatal("NewCachedClient returned nil")
	}
}

func TestCachedClient_DisabledCache(t *testing.T) {
	mock := &mockClient{
		checkFn: func(_ context.Context, req CheckRequest) (bool, error) {
			if req.User == "user:1" {
				return true, nil
			}
			return false, nil
		},
	}
	cc := NewCachedClient(mock, &CacheConfig{Enabled: false})

	allowed, err := cc.Check(context.Background(), CheckRequest{User: "user:1", Relation: "view", Object: "doc:1"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !allowed {
		t.Error("expected allowed=true with disabled cache")
	}
}

func TestCachedClient_CacheHit(t *testing.T) {
	backend := &memBackend{}
	mock := &mockClient{
		checkFn: func(_ context.Context, _ CheckRequest) (bool, error) { return true, nil },
	}

	cc := NewCachedClient(mock, &CacheConfig{
		Enabled: true,
		TTL:     time.Minute,
		Backend: backend,
	})

	req := CheckRequest{User: "user:hit", Relation: "read", Object: "doc:hit-test"}
	allowed, err := cc.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("first check failed: %v", err)
	}
	if !allowed {
		t.Error("first check expected true")
	}

	allowed, err = cc.Check(context.Background(), req)
	if err != nil {
		t.Fatalf("second check failed: %v", err)
	}
	if !allowed {
		t.Error("second check (cached) expected true")
	}

	if len(mock.callLog) != 1 {
		t.Errorf("expected 1 call to underlying client (cache hit on second), got %d", len(mock.callLog))
	}
}

func TestCachedClient_CacheMiss(t *testing.T) {
	backend := &memBackend{}
	mock := &mockClient{
		checkFn: func(_ context.Context, req CheckRequest) (bool, error) {
			if req.User == "user:deny" {
				return false, nil
			}
			return true, nil
		},
	}

	cc := NewCachedClient(mock, &CacheConfig{
		Enabled: true,
		TTL:     time.Minute,
		Backend: backend,
	})

	denied, err := cc.Check(context.Background(), CheckRequest{User: "user:deny", Relation: "write", Object: "secret:1"})
	if err != nil {
		t.Fatal(err)
	}
	if denied {
		t.Error("expected false for user:deny")
	}

	allowed, err := cc.Check(context.Background(), CheckRequest{User: "user:allow", Relation: "read", Object: "public:1"})
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Error("expected true for user:allow")
	}

	if len(mock.callLog) != 2 {
		t.Errorf("expected 2 calls to underlying client, got %d", len(mock.callLog))
	}
}

func TestCacheKeyFormatMock(t *testing.T) {
	req := CheckRequest{User: "user:1", Relation: "can_read", Object: "doc:42"}
	key := cacheKey(req)
	expected := "fga:check:user:1:can_read:doc:42"
	if key != expected {
		t.Errorf("cacheKey() = %q, want %q", key, expected)
	}
}

func TestNATSKVBackend_Close(t *testing.T) {
	b := NewNATSKVBackend(nil, nil)
	err := b.Close()
	if err != nil {
		t.Fatalf("NATSKVBackend.Close failed: %v", err)
	}
}

func TestRedisKVBackend_Close(t *testing.T) {
	b := NewRedisKVBackend(nil, nil)
	err := b.Close()
	if err != nil {
		t.Fatalf("RedisKVBackend.Close failed: %v", err)
	}
}
