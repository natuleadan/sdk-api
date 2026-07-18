package iox

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRedirectInOut(t *testing.T) {
	restore, err := RedirectInOut()
	require.NoError(t, err)
	defer restore()
}
