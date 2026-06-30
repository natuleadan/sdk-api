package middleware

import (
	"sync"

	"github.com/gofiber/fiber/v2"
	xrate "golang.org/x/time/rate"
)

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
	global  *xrate.Limiter
	perIP   map[string]*xrate.Limiter
	perUser map[string]*xrate.Limiter
}

func newRateLimiterStore() *rateLimiterStore {
	return &rateLimiterStore{
		perIP:   make(map[string]*xrate.Limiter),
		perUser: make(map[string]*xrate.Limiter),
	}
}

func RateLimit(cfg RateLimitConfig) fiber.Handler {
	store := newRateLimiterStore()
	if cfg.Global != nil && cfg.Global.RequestsPerSecond > 0 {
		store.global = xrate.NewLimiter(
			xrate.Limit(cfg.Global.RequestsPerSecond),
			cfg.Global.Burst,
		)
	}

	return func(c *fiber.Ctx) error {
		// Global limit
		if store.global != nil {
			if !store.global.Allow() {
				return rateLimitResponse(c)
			}
		}

		// Per-IP limit
		if cfg.PerIP != nil && cfg.PerIP.RequestsPerSecond > 0 {
			ip := c.IP()
			limiter := getOrCreateLimiter(store, "ip", ip, cfg.PerIP)
			if !limiter.Allow() {
				return rateLimitResponse(c)
			}
		}

		// Per-user limit (extracts user from JWT claims if available)
		if cfg.PerUser != nil && cfg.PerUser.RequestsPerSecond > 0 {
			userID := extractUserID(c)
			if userID != "" {
				limiter := getOrCreateLimiter(store, "user", userID, cfg.PerUser)
				if !limiter.Allow() {
					return rateLimitResponse(c)
				}
			}
		}

		return c.Next()
	}
}

func getOrCreateLimiter(store *rateLimiterStore, prefix, key string, entry *RateLimitEntry) *xrate.Limiter {
	store.mu.Lock()
	defer store.mu.Unlock()

	var m map[string]*xrate.Limiter
	if prefix == "user" {
		m = store.perUser
	} else {
		m = store.perIP
	}

	if l, ok := m[key]; ok {
		return l
	}
	l := xrate.NewLimiter(xrate.Limit(entry.RequestsPerSecond), entry.Burst)
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


