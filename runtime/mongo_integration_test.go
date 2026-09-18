//go:build integration

package runtime

import (
	"context"
	"os"
	"testing"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/stores/mon"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type mongoWidget struct {
	ID       string `db:"id,primary"      bson:"id"`
	Slug     string `db:"slug,unique"     bson:"slug"`
	TenantID string `db:"tenant_id,index" bson:"tenant_id"`
	Name     string `db:"name"            bson:"name"`
}

// TestMongoAutoInit_Indexes checks the Mongo "autoinit" contract against a real
// server: MongoDB has no DDL, so the equivalent is ensuring the indexes derived
// from the model (db.IndexFields) plus a unique index on the lookup field.
func TestMongoAutoInit_Indexes(t *testing.T) {
	uri := os.Getenv("MONGO_URL")
	if uri == "" {
		t.Skip("MONGO_URL not set")
	}
	ctx := context.Background()
	model, err := mon.NewModel(uri, "sdk_api_it", "widgets")
	require.NoError(t, err)
	require.NoError(t, model.RawCollection().Drop(ctx))
	t.Cleanup(func() {
		_ = model.RawCollection().Drop(ctx)
		_ = mon.Disconnect(uri)
	})

	fields, err := db.IndexFields[mongoWidget]()
	require.NoError(t, err)
	assert.Equal(t, []string{"id", "slug", "tenant_id"}, fields)

	// Mirrors MongoMustRegister: the lookup field is unique, the rest are
	// plain indexes.
	require.NoError(t, model.EnsureIndex(ctx, "slug"))
	for _, f := range fields {
		if f == "slug" {
			continue
		}
		require.NoError(t, model.EnsureIndexField(ctx, f, false))
	}

	unique := mongoIndexUnique(t, ctx, model)
	assert.True(t, unique["slug_1"], "slug index must be unique")
	assert.False(t, unique["tenant_id_1"], "tenant_id index must not be unique")

	// A unique index rejects duplicates.
	_, err = model.InsertOneNoBreaker(ctx, bson.M{"slug": "s1", "tenant_id": "t1"})
	require.NoError(t, err)
	_, err = model.InsertOneNoBreaker(ctx, bson.M{"slug": "s1", "tenant_id": "t2"})
	assert.Error(t, err, "duplicate slug must be rejected")

	// A plain index allows repeated values.
	_, err = model.InsertOneNoBreaker(ctx, bson.M{"slug": "s2", "tenant_id": "t1"})
	assert.NoError(t, err)
}

func mongoIndexUnique(t *testing.T, ctx context.Context, model *mon.Model) map[string]bool {
	t.Helper()
	cur, err := model.RawCollection().Indexes().List(ctx)
	require.NoError(t, err)
	var docs []bson.M
	require.NoError(t, cur.All(ctx, &docs))
	out := make(map[string]bool, len(docs))
	for _, d := range docs {
		name, _ := d["name"].(string)
		if name == "" {
			continue
		}
		uniq, _ := d["unique"].(bool)
		out[name] = uniq
	}
	return out
}
