package etcd

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

type Store struct {
	client *clientv3.Client
}

type Config struct {
	Endpoints []string
	Username  string
	Password  string
	TLS       bool
	Timeout   time.Duration
}

// New creates an etcd KV store connected to the given endpoints.
func New(cfg Config) (*Store, error) {
	c, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.Endpoints,
		Username:    cfg.Username,
		Password:    cfg.Password,
		DialTimeout: cfg.Timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("etcd: connect: %w", err)
	}
	return &Store{client: c}, nil
}

// Get retrieves the value for the given key.
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	resp, err := s.client.Get(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("etcd: get %s: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		return nil, nil
	}
	return resp.Kvs[0].Value, nil
}

// Put sets the value for the given key.
func (s *Store) Put(ctx context.Context, key string, value []byte) error {
	_, err := s.client.Put(ctx, key, string(value))
	if err != nil {
		return fmt.Errorf("etcd: put %s: %w", key, err)
	}
	return nil
}

// Delete removes the given key.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.client.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("etcd: delete %s: %w", key, err)
	}
	return nil
}

// DeleteWithPrefix removes all keys with the given prefix.
func (s *Store) DeleteWithPrefix(ctx context.Context, prefix string) error {
	_, err := s.client.Delete(ctx, prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("etcd: delete prefix %s: %w", prefix, err)
	}
	return nil
}

// Close closes the underlying etcd connection.
func (s *Store) Close() error {
	return s.client.Close()
}

// Keys returns all keys matching the given prefix.
func (s *Store) Keys(ctx context.Context, prefix string) ([]string, error) {
	resp, err := s.client.Get(ctx, prefix, clientv3.WithPrefix(), clientv3.WithKeysOnly())
	if err != nil {
		return nil, fmt.Errorf("etcd: keys %s: %w", prefix, err)
	}
	result := make([]string, len(resp.Kvs))
	for i, kv := range resp.Kvs {
		result[i] = string(kv.Key)
	}
	return result, nil
}
