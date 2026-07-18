package load

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNopShedder(t *testing.T) {
	Disable()
	shedder := NewAdaptiveShedder()
	for range 1000 {
		p, err := shedder.Allow()
		require.NoError(t, err)
		p.Fail()
	}

	p, err := shedder.Allow()
	require.NoError(t, err)
	p.Pass()
}
