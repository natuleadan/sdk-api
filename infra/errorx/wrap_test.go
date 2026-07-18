package errorx

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrap(t *testing.T) {
	assert.NoError(t, Wrap(nil, "test"))
	assert.Equal(t, "foo: bar", Wrap(errors.New("bar"), "foo").Error())

	err := errors.New("foo")
	assert.ErrorIs(t, Wrap(err, "bar"), err)
}

func TestWrapf(t *testing.T) {
	assert.NoError(t, Wrapf(nil, "%s", "test"))
	assert.Equal(t, "foo bar: quz", Wrapf(errors.New("quz"), "foo %s", "bar").Error())

	err := errors.New("foo")
	assert.ErrorIs(t, Wrapf(err, "foo %s", "bar"), err)
}
