package iox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedirectInOut(t *testing.T) {
	restore, err := RedirectInOut()
	assert.NoError(t, err)
	defer restore()
}
