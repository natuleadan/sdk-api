package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type indexModel struct {
	ID    string `db:"id,primary"`
	Email string `db:"email,unique"`
	Name  string `db:"name,index"`
	Bio   string `db:"bio"`
}

func TestIndexFields(t *testing.T) {
	got, err := IndexFields[indexModel]()
	require.NoError(t, err)
	assert.Equal(t, []string{"id", "email", "name"}, got)
}

func TestIndexFields_NotStruct(t *testing.T) {
	_, err := IndexFields[string]()
	assert.Error(t, err)
}
