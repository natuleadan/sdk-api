package redis

import (
	"context"
	"fmt"
)

// ScanKeys iterates over Redis keys matching a pattern using SCAN (non-blocking).
// Returns all matching keys. For large key spaces, use the ScannedCallback variant.
func ScanKeys(ctx context.Context, rdb *Redis, pattern string, count int) ([]string, error) {
	var keys []string
	var cursor uint64
	for {
		batch, next, err := rdb.Scan(cursor, pattern, int64(count))
		if err != nil {
			return nil, fmt.Errorf("redis scan: %w", err)
		}
		keys = append(keys, batch...)
		if next == 0 {
			break
		}
		cursor = next
	}
	return keys, nil
}

// ScanKeysCallback iterates over Redis keys matching a pattern using SCAN,
// calling fn for each batch of keys. Useful for large key spaces to avoid
// loading all keys into memory.
func ScanKeysCallback(ctx context.Context, rdb *Redis, pattern string, count int, fn func(keys []string) error) error {
	var cursor uint64
	for {
		batch, next, err := rdb.Scan(cursor, pattern, int64(count))
		if err != nil {
			return fmt.Errorf("redis scan: %w", err)
		}
		if len(batch) > 0 {
			if err := fn(batch); err != nil {
				return err
			}
		}
		if next == 0 {
			break
		}
		cursor = next
	}
	return nil
}
