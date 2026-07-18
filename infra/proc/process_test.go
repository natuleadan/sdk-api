package proc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessName(t *testing.T) {
	assert.NotEmpty(t, ProcessName())
}

func TestPid(t *testing.T) {
	assert.Positive(t, Pid())
}
