package middleware

import (
	"strconv"
	"sync"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/limit"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	xrate "golang.org/x/time/rate"
)

type limiter interface {
	Allow() bool
	Remaining() int
}

type xrateLimiter struct {
	*xrate.Limiter
}

func (l *xrateLimiter) Remaining() int {
	return max(0, int(l.Tokens()))
}

type redisLimiter struct {
	*limit.TokenLimiter
}

func (l *redisLimiter) Remaining() int {
	return 0
}

type RateLimitConfig struct {
	Enabled       bool            `json:"enabled,optional"`
	Driver        string          `json:"driver,default=memory"`
	RedisURL      string          `json:"redis_url,optional"`
	Global        *RateLimitEntry `json:"global,optional"`
	PerIP         *RateLimitEntry `json:"per_ip,optional"`
	PerUser       *RateLimitEntry `json:"per_user,optional"`
}

type RateLimitEntry struct {
	RequestsPerSecond int  `json:"requests_per_second"`
	Burst             int  `json:"burst"`
}

type rateLimiterStore struct {
	mu      sync.Mutex
	global  limiter
	perIP   map[string]limiter
	perUser map[string]limiter
	rdb     *redis.Redis
}

func newRateLimiterStore(driver, redisURL string) *rateLimiterStore {
	s := &rateLimiterStore{
		perIP:   make(map[string]limiter),
		perUser: make(map[string]limiter),
	}
	if driver == "redis" && redisURL != "" {
		s.rdb = redis.New(redisURL)
	}
	return s
}

func RateLimit(cfg RateLimitConfig) fiber.Handler {
	store := newRateLimiterStore(cfg.Driver, cfg.RedisURL)
	if cfg.Global != nil && cfg.Global.RequestsPerSecond > 0 {
		store.global = newLimiter(cfg.Driver, "sdk:rl:global", cfg.Global, store.rdb)
	}

	return func(c *fiber.Ctx) error {
		var limit, remaining int

		if store.global != nil {
			r := store.global.Remaining()
			if !store.global.Allow() {
				setRateLimitHeaders(c, cfg.Global.RequestsPerSecond, r)
				return rateLimitResponse(c)
			}
			limit = cfg.Global.RequestsPerSecond
			remaining = r
		}

		if cfg.PerIP != nil && cfg.PerIP.RequestsPerSecond > 0 {
			ip := c.IP()
			l := getOrCreateLimiter(store, "ip", ip, cfg.PerIP)
			r := l.Remaining()
			if !l.Allow() {
				setRateLimitHeaders(c, cfg.PerIP.RequestsPerSecond, r)
				return rateLimitResponse(c)
			}
			limit = cfg.PerIP.RequestsPerSecond
			remaining = r
		}

		if cfg.PerUser != nil && cfg.PerUser.RequestsPerSecond > 0 {
			userID := extractUserID(c)
			if userID != "" {
				l := getOrCreateLimiter(store, "user", userID, cfg.PerUser)
				r := l.Remaining()
				if !l.Allow() {
					setRateLimitHeaders(c, cfg.PerUser.RequestsPerSecond, r)
					return rateLimitResponse(c)
				}
				limit = cfg.PerUser.RequestsPerSecond
				remaining = r
			}
		}

		setRateLimitHeaders(c, limit, remaining)
		return c.Next()
	}
}

func newLimiter(driver, key string, entry *RateLimitEntry, rdb *redis.Redis) limiter {
	if driver == "redis" && rdb != nil {
		return &redisLimiter{limit.NewTokenLimiter(entry.RequestsPerSecond, entry.Burst, rdb, key)}
	}
	return &xrateLimiter{xrate.NewLimiter(xrate.Limit(entry.RequestsPerSecond), entry.Burst)}
}

func setRateLimitHeaders(c *fiber.Ctx, limit, remaining int) {
	if limit > 0 {
		c.Set("X-RateLimit-Limit", strconv.Itoa(limit))
	}
	c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
}

func getOrCreateLimiter(store *rateLimiterStore, prefix, key string, entry *RateLimitEntry) limiter {
	store.mu.Lock()
	defer store.mu.Unlock()

	m := store.perUser
	if prefix == "ip" {
		if store.perIP == nil {
			store.perIP = make(map[string]limiter)
		}
		m = store.perIP
	} else {
		if store.perUser == nil {
			store.perUser = make(map[string]limiter)
		}
	}

	if l, ok := m[key]; ok {
		return l
	}
	rlKey := "sdk:rl:" + prefix + ":" + key
	driver := "memory"
	if store.rdb != nil {
		driver = "redis"
	}
	l := newLimiter(driver, rlKey, entry, store.rdb)
	m[key] = l
	return l
}

func extractUserID(c *fiber.Ctx) string {
	if claims := c.Locals("claims"); claims != nil {
		if m, ok := claims.(map[string]any); ok {
			if sub, ok := m["sub"]; ok {
				if s, ok := sub.(string); ok {
					return s
				}
			}
		}
	}
	return ""
}

func rateLimitResponse(c *fiber.Ctx) error {
	c.Set("Retry-After", "1")
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"code":    429,
		"message": "rate limit exceeded",
	})
}
