package syncx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefCleaner(t *testing.T) {
	var count int
	clean := func() {
		count += 1
	}

	cleaner := NewRefResource(clean)
	err := cleaner.Use()
	require.NoError(t, err)
	err = cleaner.Use()
	require.NoError(t, err)
	cleaner.Clean()
	cleaner.Clean()
	assert.InDelta(t, 1, count, 0.01)
	cleaner.Clean()
	cleaner.Clean()
	assert.InDelta(t, 1, count, 0.01)
	assert.Equal(t, ErrUseOfCleaned, cleaner.Use())
}
