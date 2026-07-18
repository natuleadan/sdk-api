package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUuid(t *testing.T) {
	assert.Len(t, NewUuid(), 36)
}
