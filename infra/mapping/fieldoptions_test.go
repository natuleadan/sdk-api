package mapping

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type Bar struct {
	Val string `json:"val"`
}

func TestFieldOptionOptionalDep(t *testing.T) {
	rt := reflect.TypeFor[Bar]()
	for field := range rt.Fields() {
		val, opt, err := parseKeyAndOptions(jsonTagKey, field)
		assert.Equal(t, "val", val)
		assert.Nil(t, opt)
		require.NoError(t, err)
	}

	// check nil working
	var o *fieldOptions
	check := func(o *fieldOptions) {
		assert.Empty(t, o.optionalDep())
	}
	check(o)
}
