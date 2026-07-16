package middleware

import (
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/limit"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
	xrate "golang.org/x/time/rate"
)

type limiter interface {
	Allow() bool
	Remaining() int
}

type cancellableLimiter interface {
	Cancel()
}

type xrateLimiter struct {
	*xrate.Limiter
	mu    sync.Mutex
	owed  int
}

func (l *xrateLimiter) Remaining() int {
	return max(0, int(l.Tokens()))
}

func (l *xrateLimiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.owed > 0 {
		l.owed--
		return true
	}
	return l.Limiter.Allow()
}

func (l *xrateLimiter) Cancel() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.owed++
}

type redisLimiter struct {
	*limit.TokenLimiter
}

func (l *redisLimiter) Remaining() int {
	return 0
}

func (l *redisLimiter) Cancel() {}

const (
	AlgorithmTokenBucket = "token_bucket"
	AlgorithmSlidingWindow = "sliding_window"
)

type RateLimitConfig struct {
	Enabled              bool              `json:"enabled" config:",optional"`
	Global               *RateLimitEntry   `json:"global" config:",optional"`
	PerIP                *RateLimitEntry   `json:"per_ip" config:",optional"`
	Algorithm            string            `json:"algorithm" config:",default=sliding_window"`
	TTL                  time.Duration     `json:"ttl" config:",optional"`
	SkipFailedRequests   bool              `json:"skip_failed_requests" config:",optional"`
	SkipSuccessfulRequests bool            `json:"skip_successful_requests" config:",optional"`
	MaxFunc              func(c fiber.Ctx) int `json:"-" config:"-"`
	RedisConn            *redis.Redis      `json:"-" config:"-"`
}

type RateLimitEntry struct {
	RequestsPerSecond int           `json:"requests_per_second"`
	Burst             int           `json:"burst"`
	TTL               time.Duration `json:"ttl" config:",optional"`
}

type rateLimitEntryState struct {
	limiter   limiter
	expiresAt int64
}

type rateLimiterStore struct {
	mu       sync.Mutex
	global   limiter
	perIP    map[string]*rateLimitEntryState
	perUser  map[string]*rateLimitEntryState
	perKey   map[string]*rateLimitEntryState
	rdb      *redis.Redis
	alg      string
	ttl      time.Duration
	gcTick   time.Duration
	gcDone   chan struct{}
}

func newRateLimiterStore(alg string, ttl time.Duration, rdb ...*redis.Redis) *rateLimiterStore {
	s := &rateLimiterStore{
		perIP:   make(map[string]*rateLimitEntryState),
		perUser: make(map[string]*rateLimitEntryState),
		perKey:  make(map[string]*rateLimitEntryState),
		alg:     alg,
		ttl:     ttl,
		gcDone:  make(chan struct{}),
		gcTick:  ttl,
	}
	if s.gcTick <= 0 {
		s.gcTick = 5 * time.Minute
	}
	if len(rdb) > 0 {
		s.rdb = rdb[0]
	}
	go s.gcLoop()
	return s
}

func (s *rateLimiterStore) gcLoop() {
	ticker := time.NewTicker(s.gcTick)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.gc()
		case <-s.gcDone:
			return
		}
	}
}

func (s *rateLimiterStore) gc() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UnixNano()
	for k, v := range s.perIP {
		if v.expiresAt > 0 && now >= v.expiresAt {
			delete(s.perIP, k)
		}
	}
	for k, v := range s.perUser {
		if v.expiresAt > 0 && now >= v.expiresAt {
			delete(s.perUser, k)
		}
	}
	for k, v := range s.perKey {
		if v.expiresAt > 0 && now >= v.expiresAt {
			delete(s.perKey, k)
		}
	}
}

func shouldSkipRequest(cfg RateLimitConfig, c fiber.Ctx) bool {
	if cfg.SkipFailedRequests || cfg.SkipSuccessfulRequests {
		if c.Response() == nil {
			return false
		}
		code := c.Response().StatusCode()
		if cfg.SkipFailedRequests && code >= 400 {
			return true
		}
		if cfg.SkipSuccessfulRequests && code < 400 {
			return true
		}
	}
	return false
}

func cancelUsed(limiters []cancellableLimiter) {
	for _, l := range limiters {
		l.Cancel()
	}
}

func isCancellable(l limiter) cancellableLimiter {
	if c, ok := l.(cancellableLimiter); ok {
		return c
	}
	return nil
}

func RateLimit(cfg RateLimitConfig) fiber.Handler {
	alg := cfg.Algorithm
	if alg == "" {
		alg = AlgorithmSlidingWindow
	}

	store := newRateLimiterStore(alg, cfg.TTL, cfg.RedisConn)
	if cfg.Global != nil && cfg.Global.RequestsPerSecond > 0 {
		store.global = newLimiter("sdk:rl:global", cfg.Global, store.rdb, alg)
	}

	return func(c fiber.Ctx) error {
		var limit, remaining int
		var used []cancellableLimiter

		if store.global != nil {
			if !store.global.Allow() {
				r := store.global.Remaining()
				setRateLimitHeaders(c, cfg.Global.RequestsPerSecond, r)
				return rateLimitResponse(c)
			}
			if cl := isCancellable(store.global); cl != nil {
				used = append(used, cl)
			}
			limit = cfg.Global.RequestsPerSecond
			remaining = store.global.Remaining()
		}

		if cfg.PerIP != nil && cfg.PerIP.RequestsPerSecond > 0 {
			l := getOrCreateLimiter(store, "ip", c.IP(), cfg.PerIP)
			if !l.Allow() {
				r := l.Remaining()
				setRateLimitHeaders(c, cfg.PerIP.RequestsPerSecond, r)
				return rateLimitResponse(c)
			}
			if cl := isCancellable(l); cl != nil {
				used = append(used, cl)
			}
			limit = cfg.PerIP.RequestsPerSecond
			remaining = l.Remaining()
		}

		setRateLimitHeaders(c, limit, remaining)
		err := c.Next()

		if len(used) > 0 && err == nil && shouldSkipRequest(cfg, c) {
			cancelUsed(used)
		}

		return err
	}
}

func newLimiter(key string, entry *RateLimitEntry, rdb *redis.Redis, alg string) limiter {
	if rdb != nil {
		return &redisLimiter{limit.NewTokenLimiter(entry.RequestsPerSecond, entry.Burst, rdb, key)}
	}
	switch alg {
	case AlgorithmSlidingWindow:
		return newSlidingWindowLimiter(entry.RequestsPerSecond, entry.Burst)
	default:
		return &xrateLimiter{Limiter: xrate.NewLimiter(xrate.Limit(entry.RequestsPerSecond), entry.Burst)}
	}
}

func setRateLimitHeaders(c fiber.Ctx, limit, remaining int) {
	if limit > 0 {
		c.Set("X-RateLimit-Limit", strconv.Itoa(limit))
	}
	if remaining < 0 {
		remaining = 0
	}
	c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
}

func getOrCreateLimiter(store *rateLimiterStore, prefix, key string, entry *RateLimitEntry) limiter {
	store.mu.Lock()
	defer store.mu.Unlock()

	var m map[string]*rateLimitEntryState
	switch prefix {
	case "ip":
		m = store.perIP
	case "key", "entry_key":
		m = store.perKey
	default:
		m = store.perUser
	}

	now := time.Now().UnixNano()
	if es, ok := m[key]; ok {
		if es.expiresAt == 0 || now < es.expiresAt {
			return es.limiter
		}
		delete(m, key)
	}

	rlKey := "sdk:rl:" + prefix + ":" + key
	l := newLimiter(rlKey, entry, store.rdb, store.alg)

	ttl := entry.TTL
	if ttl <= 0 {
		ttl = store.ttl
	}
	var expiresAt int64
	if ttl > 0 {
		expiresAt = now + ttl.Nanoseconds()
	}

	m[key] = &rateLimitEntryState{limiter: l, expiresAt: expiresAt}
	return l
}

func extractUserID(c fiber.Ctx) string {
	if claims := c.Locals("claims"); claims != nil {
		if m, ok := claims.(map[string]any); ok {
			if sub, ok := m["sub"]; ok {
				if s, ok := sub.(string); ok {
					return s
				}
			}
		}
	}
	if auth := GetAuth(c); auth != nil {
		return auth.UserID
	}
	return ""
}

func extractKeyID(c fiber.Ctx) string {
	if auth := GetAuth(c); auth != nil && auth.RawToken == "" {
		return auth.UserID
	}
	return ""
}

func rateLimitResponse(c fiber.Ctx) error {
	c.Set("Retry-After", "1")
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"code":    429,
		"message": "rate limit exceeded",
	})
}

// --- Post-auth rate limiting ---

type RateLimitPostConfig struct {
	ServerPerUser *RateLimitEntry
	ServerPerKey  *RateLimitEntry
	EntryPerUser  *RateLimitEntry
	EntryPerKey   *RateLimitEntry
	PerRoleLimits map[string]*RateLimitEntry
	MaxFunc       func(c fiber.Ctx) int
	Algorithm     string
	TTL           time.Duration
	RedisConn     *redis.Redis
}

type postLimitResult struct {
	blocked bool
	limiter limiter
}

func resolvePostEntry(entry *RateLimitEntry, c fiber.Ctx, maxFunc func(c fiber.Ctx) int) *RateLimitEntry {
	if entry == nil {
		return nil
	}
	if maxFunc != nil {
		if m := maxFunc(c); m > 0 {
			return &RateLimitEntry{
				RequestsPerSecond: m,
				Burst:             m,
				TTL:               entry.TTL,
			}
		}
	}
	return entry
}

func checkPostLimitWithCancel(store *rateLimiterStore, c fiber.Ctx, entry *RateLimitEntry, prefix string, extract func(fiber.Ctx) string, maxFunc func(c fiber.Ctx) int) postLimitResult {
	entry = resolvePostEntry(entry, c, maxFunc)
	if entry == nil || entry.RequestsPerSecond <= 0 {
		return postLimitResult{blocked: false}
	}
	id := extract(c)
	if id == "" {
		return postLimitResult{blocked: false}
	}
	l := getOrCreateLimiter(store, prefix, id, entry)
	return postLimitResult{blocked: !l.Allow(), limiter: l}
}

func checkRoleLimitsWithCancel(store *rateLimiterStore, c fiber.Ctx, roleLimits map[string]*RateLimitEntry) postLimitResult {
	if len(roleLimits) == 0 {
		return postLimitResult{blocked: false}
	}
	auth := GetAuth(c)
	if auth == nil {
		return postLimitResult{blocked: false}
	}
	for _, role := range auth.Roles {
		if entry, ok := roleLimits[role]; ok {
			if entry != nil && entry.RequestsPerSecond > 0 {
				l := getOrCreateLimiter(store, "role", role, entry)
				if !l.Allow() {
					return postLimitResult{blocked: true, limiter: l}
				}
			}
		}
	}
	return postLimitResult{blocked: false}
}

func RateLimitPost(cfg RateLimitPostConfig) fiber.Handler {
	alg := cfg.Algorithm
	if alg == "" {
		alg = AlgorithmSlidingWindow
	}

	store := newRateLimiterStore(alg, cfg.TTL, cfg.RedisConn)

	return func(c fiber.Ctx) error {
		var blocked bool

		check := func(entry *RateLimitEntry, prefix string, extract func(fiber.Ctx) string, maxFunc func(c fiber.Ctx) int) bool {
			if blocked {
				return true
			}
			r := checkPostLimitWithCancel(store, c, entry, prefix, extract, maxFunc)
			if r.blocked {
				blocked = true
			}
			return r.blocked
		}

		check(nil, "", nil, nil) // ensure blocked is false
		blocked = false

		if check(cfg.ServerPerUser, "user", extractUserID, cfg.MaxFunc) {
			return rateLimitResponse(c)
		}
		if check(cfg.ServerPerKey, "key", extractKeyID, cfg.MaxFunc) {
			return rateLimitResponse(c)
		}
		if checkRoleLimitsWithCancel(store, c, cfg.PerRoleLimits).blocked {
			return rateLimitResponse(c)
		}
		if check(cfg.EntryPerUser, "entry_user", extractUserID, cfg.MaxFunc) {
			return rateLimitResponse(c)
		}
		if check(cfg.EntryPerKey, "entry_key", extractKeyID, cfg.MaxFunc) {
			return rateLimitResponse(c)
		}

		return c.Next()
	}
}
