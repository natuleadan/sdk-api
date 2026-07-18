package netx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInternalIp(t *testing.T) {
	assert.NotEmpty(t, InternalIp())
}
