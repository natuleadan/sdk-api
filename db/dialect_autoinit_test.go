package db

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dialectOverrideModel exercises every type override and server default the
// dialect layer translates, so AutoInit is checked against each engine and not
// only against simple int/string/float models.
type dialectOverrideModel struct {
	ID        string    `db:"id,primary,type=UUID,default=gen_random_uuid()"`
	Payload   string    `db:"payload,type=JSONB"`
	Labels    string    `db:"labels,type=TEXT[]"`
	Amount    float64   `db:"amount,type=DECIMAL(10,2),default=0"`
	CreatedAt time.Time `db:"created_at,default=now()"`
}

func TestDialectAutoInit_TursoLocal(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "dialect.db")
	table, err := NewTursoTable[dialectOverrideModel](dbPath, "dialect_override")
	require.NoError(t, err)

	require.NoError(t, table.AutoInit(ctx))
	require.NoError(t, table.AutoInit(ctx), "autoinit must be idempotent")

	count, err := table.Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)
}
