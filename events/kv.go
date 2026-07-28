package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
)

type Cache[T any] struct {
	kv  nats.KeyValue
	ttl time.Duration
}

func NewCache[T any](kv nats.KeyValue, ttl time.Duration) *Cache[T] {
	return &Cache[T]{kv: kv, ttl: ttl}
}

func (c *Cache[T]) Get(_ context.Context, key string) (*T, error) {
	entry, err := c.kv.Get(key)
	if err != nil {
		if err == nats.ErrKeyNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("events: cache get: %w", err)
	}
	var val T
	if err := json.Unmarshal(entry.Value(), &val); err != nil {
		return nil, fmt.Errorf("events: cache unmarshal: %w", err)
	}
	return &val, nil
}

func (c *Cache[T]) Set(_ context.Context, key string, val T) error {
	data, err := json.Marshal(val)
	if err != nil {
		return fmt.Errorf("events: cache marshal: %w", err)
	}
	_, err = c.kv.Put(key, data)
	if err != nil {
		return fmt.Errorf("events: cache put: %w", err)
	}
	return nil
}

func (c *Cache[T]) Delete(_ context.Context, key string) error {
	err := c.kv.Delete(key)
	if err != nil && err != nats.ErrKeyNotFound {
		return fmt.Errorf("events: cache delete: %w", err)
	}
	return nil
}

func (c *Cache[T]) GetOrSet(ctx context.Context, key string, fn func() (*T, error)) (*T, error) {
	val, err := c.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if val != nil {
		return val, nil
	}
	val, err = fn()
	if err != nil {
		return nil, err
	}
	if val != nil {
		if setErr := c.Set(ctx, key, *val); setErr != nil {
			return nil, setErr
		}
	}
	return val, nil
}
