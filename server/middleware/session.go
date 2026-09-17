package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/stores/redis"
)

// SessionConfig validates a session cookie against a server-side store.
// Unlike JWT cookies (stateless), sessions can be revoked individually.
type SessionConfig struct {
	// Cookie is the session cookie name (default "sid").
	Cookie string
	// Store holds sessions as sess:<id> JSON with TTL expiry. Required:
	// without shared storage, sessions break under prefork/clustering.
	Store *redis.Redis
	// TTL is the session lifetime from creation (no sliding refresh).
	TTL time.Duration
}

// sessionData is the stored session payload.
type sessionData struct {
	UserID string   `json:"user_id"`
	Roles  []string `json:"roles"`
}

func sessionKey(id string) string { return "sess:" + id }

// CreateSession stores a new session and returns its ID. The caller sets
// the cookie (name from config) with the returned ID.
func CreateSession(ctx context.Context, store *redis.Redis, ttl time.Duration, userID string, roles []string) (string, error) {
	if store == nil {
		return "", errors.New("session store not configured")
	}
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(raw[:])
	data, err := json.Marshal(sessionData{UserID: userID, Roles: roles})
	if err != nil {
		return "", err
	}
	secs := int(ttl.Seconds())
	if secs <= 0 {
		secs = 86400
	}
	if err := store.SetexCtx(ctx, sessionKey(id), string(data), secs); err != nil {
		return "", err
	}
	return id, nil
}

// DestroySession revokes a session immediately (logout).
func DestroySession(ctx context.Context, store *redis.Redis, id string) error {
	if store == nil {
		return errors.New("session store not configured")
	}
	if id == "" {
		return nil
	}
	_, err := store.DelCtx(ctx, sessionKey(id))
	return err
}

// Session validates the session cookie against the store and injects the
// resulting AuthContext for roles, per-user rate limiting and handlers.
// Missing/expired/unknown sessions get 401.
func Session(cfg SessionConfig) fiber.Handler {
	cookie := cfg.Cookie
	if cookie == "" {
		cookie = "sid"
	}
	return func(c fiber.Ctx) error {
		if cfg.Store == nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"code":    500,
				"message": "session store not configured",
			})
		}
		id := c.Cookies(cookie)
		if id == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "missing session",
			})
		}
		raw, err := cfg.Store.GetCtx(c.Context(), sessionKey(id))
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "invalid or expired session",
			})
		}
		var data sessionData
		if err := json.Unmarshal([]byte(raw), &data); err != nil || data.UserID == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    401,
				"message": "invalid session",
			})
		}
		injectAuth(c, &AuthContext{UserID: data.UserID, Roles: data.Roles})
		return c.Next()
	}
}
