package redis

import (
	"crypto/tls"
	"io"
	"runtime"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/syncx"
	red "github.com/redis/go-redis/v9"
)

const (
	maxRetries = 3
	idleConns  = 8
)

var (
	clientManager = syncx.NewResourceManager()
	// nodePoolSize is default pool size for node type of redis.
	nodePoolSize = 10 * runtime.GOMAXPROCS(0)
)

func getClient(r *Redis) (*red.Client, error) {
	val, err := clientManager.GetResource(r.Addr, func() (io.Closer, error) {
		var tlsConfig *tls.Config
		if r.tls {
			tlsConfig = &tls.Config{}
			if r.tlsSkipVerify {
				tlsConfig.InsecureSkipVerify = true
				logx.Errorf("redis: TLS InsecureSkipVerify enabled for %s (node)", r.Addr)
			}
		}
		store := red.NewClient(&red.Options{
			Addr:         r.Addr,
			Username:     r.User,
			Password:     r.Pass,
			DB:           r.DB,
			MaxRetries:   maxRetries,
			MinIdleConns: idleConns,
			TLSConfig:    tlsConfig,
		})

		hooks := append([]red.Hook{defaultDurationHook, breakerHook{
			brk: r.brk,
		}}, r.hooks...)
		for _, hook := range hooks {
			store.AddHook(hook)
		}

		connCollector.registerClient(&statGetter{
			clientType: NodeType,
			key:        r.Addr,
			poolSize:   nodePoolSize,
			poolStats: func() *red.PoolStats {
				return store.PoolStats()
			},
		})

		return store, nil
	})
	if err != nil {
		return nil, err
	}

	return val.(*red.Client), nil
}
