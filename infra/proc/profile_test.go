package proc

import (
	"testing"

	"github.com/natuleadan/sdk-api/infra/logx/logtest"
	"github.com/stretchr/testify/assert"
)

func TestProfile(t *testing.T) {
	c := logtest.NewCollector(t)
	profiler := StartProfile()
	// start again should not work
	assert.NotNil(t, StartProfile())
	profiler.Stop()
	// stop twice
	profiler.Stop()
	assert.Contains(t, c.String(), ".pprof")
}
