//go:build integration

package db

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDialectAutoInit_Postgres(t *testing.T) {
	if os.Getenv("DATABASE_URL") == "" {
		t.Skip("DATABASE_URL not set")
	}
	ctx := context.Background()
	table, err := NewTable[dialectOverrideModel](testPool(t), "dialect_override_pg")
	require.NoError(t, err)

	require.NoError(t, table.AutoInit(ctx))
	require.NoError(t, table.AutoInit(ctx), "autoinit must be idempotent")

	count, err := table.Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)
}

func TestDialectAutoInit_MySQL(t *testing.T) {
	url := os.Getenv("MYSQL_URL")
	if url == "" {
		t.Skip("MYSQL_URL not set")
	}
	ctx := context.Background()
	table, err := NewMySQLTableFromURL[dialectOverrideModel](url, "dialect_override_mysql")
	require.NoError(t, err)

	require.NoError(t, table.AutoInit(ctx))
	require.NoError(t, table.AutoInit(ctx), "autoinit must be idempotent")

	count, err := table.Count(ctx)
	require.NoError(t, err)
	assert.EqualValues(t, 0, count)
}
