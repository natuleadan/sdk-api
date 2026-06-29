package redis

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func TestScriptCache(t *testing.T) {
	logx.Disable()

	cache := GetScriptCache()
	cache.SetSha("foo", "bar")
	cache.SetSha("bla", "blabla")
	bar, ok := cache.GetSha("foo")
	assert.True(t, ok)
	assert.Equal(t, "bar", bar)
}
