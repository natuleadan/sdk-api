package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/natuleadan/sdk-api/events"
)

type natsKV struct {
	conn        *events.Conn
	bucket      string
	expiry      time.Duration
	errNotFound error
}

func NewNATSCache(conn *events.Conn, bucket string, expiry time.Duration) Cache {
	return &natsKV{
		conn:        conn,
		bucket:      bucket,
		expiry:      expiry,
		errNotFound: fmt.Errorf("nats kv: key not found"),
	}
}

func (n *natsKV) Del(keys ...string) error {
	return n.DelCtx(context.Background(), keys...)
}

func (n *natsKV) DelCtx(ctx context.Context, keys ...string) error {
	var errs []error
	for _, key := range keys {
		if err := n.conn.KVDelete(n.bucket, key); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("nats kv del: %v", errs)
	}
	return nil
}

func (n *natsKV) Get(key string, val any) error {
	return n.GetCtx(context.Background(), key, val)
}

func (n *natsKV) GetCtx(ctx context.Context, key string, val any) error {
	data, err := n.conn.KVGet(n.bucket, key)
	if err != nil {
		return n.errNotFound
	}
	return json.Unmarshal(data, val)
}

func (n *natsKV) IsNotFound(err error) bool {
	return err != nil && err.Error() == n.errNotFound.Error()
}

func (n *natsKV) Set(key string, val any) error {
	return n.SetWithExpireCtx(context.Background(), key, val, n.expiry)
}

func (n *natsKV) SetCtx(ctx context.Context, key string, val any) error {
	return n.SetWithExpireCtx(ctx, key, val, n.expiry)
}

func (n *natsKV) SetWithExpire(key string, val any, expire time.Duration) error {
	return n.SetWithExpireCtx(context.Background(), key, val, expire)
}

func (n *natsKV) SetWithExpireCtx(ctx context.Context, key string, val any, expire time.Duration) error {
	data, err := json.Marshal(val)
	if err != nil {
		return err
	}
	_, err = n.conn.KVPut(n.bucket, key, data)
	return err
}

func (n *natsKV) Take(val any, key string, query func(val any) error) error {
	return n.TakeWithExpireCtx(context.Background(), val, key, func(v any, _ time.Duration) error {
		return query(v)
	})
}

func (n *natsKV) TakeCtx(ctx context.Context, val any, key string, query func(val any) error) error {
	return n.TakeWithExpireCtx(ctx, val, key, func(v any, _ time.Duration) error {
		return query(v)
	})
}

func (n *natsKV) TakeWithExpire(val any, key string, query func(val any, expire time.Duration) error) error {
	return n.TakeWithExpireCtx(context.Background(), val, key, query)
}

func (n *natsKV) TakeWithExpireCtx(ctx context.Context, val any, key string, query func(val any, expire time.Duration) error) error {
	if err := n.GetCtx(ctx, key, val); err == nil {
		return nil
	}
	if err := query(val, n.expiry); err != nil {
		return err
	}
	return n.SetWithExpireCtx(ctx, key, val, n.expiry)
}
