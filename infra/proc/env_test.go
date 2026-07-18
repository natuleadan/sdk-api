package proc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnv(t *testing.T) {
	assert.Empty(t, Env("any"))
	envLock.RLock()
	val, ok := envs["any"]
	envLock.RUnlock()
	assert.Empty(t, val)
	assert.True(t, ok)
	assert.Empty(t, Env("any"))
}

func TestEnvInt(t *testing.T) {
	val, ok := EnvInt("any")
	assert.Equal(t, 0, val)
	assert.False(t, ok)
	t.Setenv("anyInt", "10")
	val, ok = EnvInt("anyInt")
	assert.Equal(t, 10, val)
	assert.True(t, ok)
	t.Setenv("anyString", "a")
	val, ok = EnvInt("anyString")
	assert.Equal(t, 0, val)
	assert.False(t, ok)
}
