package manual

import (
	"context"

	"github.com/natuleadan/sdk-api/server/middleware"
)

// AuthValidator is a function that validates authorization for a request.
// It receives the AuthContext and can use any backend (DB, NATS KV, Redis, HTTP).
// Return nil if allowed, an error with message if denied.
type AuthValidator func(ctx context.Context, auth *middleware.AuthContext, entryRoles []string) error
