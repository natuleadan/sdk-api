package redis

import (
	"crypto/tls"
	"io"
	"runtime"
	"strings"

	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/syncx"
	red "github.com/redis/go-redis/v9"
)

var (
	sentinelManager  = syncx.NewResourceManager()
	sentinelPoolSize = 10 * runtime.GOMAXPROCS(0)
)

func getSentinel(r *Redis) (*red.Client, error) {
	key := r.MasterName + "@" + r.Addr
	val, err := sentinelManager.GetResource(key, func() (io.Closer, error) {
		var tlsConfig *tls.Config
		if r.tls {
			tlsConfig = &tls.Config{}
			if r.tlsSkipVerify {
				tlsConfig.InsecureSkipVerify = true
				logx.Errorf("redis: TLS InsecureSkipVerify enabled for %s (sentinel)", r.Addr)
			}
		}
		store := red.NewFailoverClient(&red.FailoverOptions{
			MasterName:       r.MasterName,
			SentinelAddrs:    splitSentinelAddrs(r.Addr),
			SentinelUsername: r.SentinelUser,
			SentinelPassword: r.SentinelPass,
			Username:         r.User,
			Password:         r.Pass,
			DB:               r.DB,
			MaxRetries:       maxRetries,
			MinIdleConns:     idleConns,
			TLSConfig:        tlsConfig,
		})

		hooks := append([]red.Hook{defaultDurationHook, breakerHook{
			brk: r.brk,
		}}, r.hooks...)
		for _, hook := range hooks {
			store.AddHook(hook)
		}

		connCollector.registerClient(&statGetter{
			clientType: SentinelType,
			key:        key,
			poolSize:   sentinelPoolSize,
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

func splitSentinelAddrs(addr string) []string {
	addrs := strings.Split(addr, ",")
	unique := make(map[string]struct{})
	for _, each := range addrs {
		a := strings.TrimSpace(each)
		if len(a) > 0 {
			unique[a] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for k := range unique {
		result = append(result, k)
	}
	return result
}
