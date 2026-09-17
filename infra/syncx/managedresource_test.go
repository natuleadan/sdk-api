package syncx

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestManagedResource(t *testing.T) {
	var count atomic.Int32
	resource := NewManagedResource(func() any {
		return count.Add(1)
	}, func(a, b any) bool {
		return a == b
	})

	val := resource.Take()
	assert.NotNil(t, val)
	old := resource.Take()
	resource.MarkBroken(old)
	assert.NotEqual(t, old, resource.Take())
}
