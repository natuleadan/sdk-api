package openfga

import (
	"context"
	"fmt"
	"time"
)

// Checker is the interface for authorization checks, optionally cached.
type Checker interface {
	Check(ctx context.Context, req CheckRequest) (bool, error)
}

// CacheBackend stores and retrieves cached check results.
type CacheBackend interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Close() error
}

// CacheConfig defines cache settings.
type CacheConfig struct {
	Enabled bool
	TTL     time.Duration
	Backend CacheBackend
}

// CachedClient wraps a Client with optional caching.
type CachedClient struct {
	client Checker
	cache  *CacheConfig
}

// NewCachedClient creates a new cached authorization client.
func NewCachedClient(client Checker, cache *CacheConfig) *CachedClient {
	return &CachedClient{client: client, cache: cache}
}

// Check performs an authorization check, using cache if configured.
func (c *CachedClient) Check(ctx context.Context, req CheckRequest) (bool, error) {
	if c.cache == nil || !c.cache.Enabled || c.cache.Backend == nil {
		return c.client.Check(ctx, req)
	}

	key := cacheKey(req)

	if data, err := c.cache.Backend.Get(ctx, key); err == nil && len(data) > 0 {
		return data[0] == '1', nil
	}

	allowed, err := c.client.Check(ctx, req)
	if err != nil {
		return false, err
	}

	val := []byte{'0'}
	if allowed {
		val = []byte{'1'}
	}
	_ = c.cache.Backend.Set(ctx, key, val, c.cache.TTL)

	return allowed, nil
}

func cacheKey(req CheckRequest) string {
	return fmt.Sprintf("fga:check:%s:%s:%s", req.User, req.Relation, req.Object)
}

// NATSKVBackend implements CacheBackend using NATS KeyValue.
type NATSKVBackend struct {
	get func(key string) ([]byte, error)
	put func(key string, value []byte) error
}

func NewNATSKVBackend(get func(string) ([]byte, error), put func(string, []byte) error) *NATSKVBackend {
	return &NATSKVBackend{get: get, put: put}
}

func (n *NATSKVBackend) Get(_ context.Context, key string) ([]byte, error) {
	return n.get(key)
}

func (n *NATSKVBackend) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	return n.put(key, value)
}

func (n *NATSKVBackend) Close() error { return nil }

// RedisKVBackend implements CacheBackend using Redis.
type RedisKVBackend struct {
	get  func(ctx context.Context, key string) (string, error)
	setex func(ctx context.Context, key, value string, seconds int) error
}

func NewRedisKVBackend(
	get func(ctx context.Context, key string) (string, error),
	setex func(ctx context.Context, key, value string, seconds int) error,
) *RedisKVBackend {
	return &RedisKVBackend{get: get, setex: setex}
}

func (r *RedisKVBackend) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.get(ctx, key)
	if err != nil {
		return nil, err
	}
	return []byte(val), nil
}

func (r *RedisKVBackend) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.setex(ctx, key, string(value), int(ttl.Seconds()))
}

func (r *RedisKVBackend) Close() error { return nil }

var (
	_ CacheBackend = (*NATSKVBackend)(nil)
	_ CacheBackend = (*RedisKVBackend)(nil)
	_ Checker      = (*CachedClient)(nil)
	_ Checker      = (*Client)(nil)
)
