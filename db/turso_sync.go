package db

import (
	"context"

	turso "turso.tech/database/tursogo"
)

// newTursoSyncDB creates a tursogo TursoSyncDb (local-first embedded database
// synced to Turso Cloud). Only compatible with Turso Cloud databases (the
// sync protocol is not implemented by Bunny).
func newTursoSyncDB(ctx context.Context, path, remoteURL, authToken string) (*turso.TursoSyncDb, error) {
	return turso.NewTursoSyncDb(ctx, turso.TursoSyncDbConfig{
		Path:      path,
		RemoteUrl: remoteURL,
		AuthToken: authToken,
	})
}
