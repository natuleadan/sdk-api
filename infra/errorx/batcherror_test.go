package errorx

import (
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	err1 = "first error"
	err2 = "second error"
)

func TestBatchErrorNil(t *testing.T) {
	var batch BatchError
	assert.NoError(t, batch.Err())
	assert.False(t, batch.NotNil())
	batch.Add(nil)
	assert.NoError(t, batch.Err())
	assert.False(t, batch.NotNil())
}

func TestBatchErrorNilFromFunc(t *testing.T) {
	err := func() error {
		var be BatchError
		return be.Err()
	}()
	assert.NoError(t, err)
}

func TestBatchErrorOneError(t *testing.T) {
	var batch BatchError
	batch.Add(errors.New(err1))
	assert.Error(t, batch.Err())
	assert.Equal(t, err1, batch.Err().Error())
	assert.True(t, batch.NotNil())
}

func TestBatchErrorWithErrors(t *testing.T) {
	var batch BatchError
	batch.Add(errors.New(err1))
	batch.Add(errors.New(err2))
	assert.Error(t, batch.Err())
	assert.Equal(t, fmt.Sprintf("%s\n%s", err1, err2), batch.Err().Error())
	assert.True(t, batch.NotNil())
}

func TestBatchErrorConcurrentAdd(t *testing.T) {
	const count = 10000
	var batch BatchError
	var wg sync.WaitGroup

	wg.Add(count)
	for range count {
		go func() {
			defer wg.Done()
			batch.Add(errors.New(err1))
		}()
	}
	wg.Wait()

	assert.Error(t, batch.Err())
	assert.Len(t, batch.errs, count)
	assert.True(t, batch.NotNil())
}

func TestBatchError_Unwrap(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var be BatchError
		assert.NoError(t, be.Err())
		assert.NoError(t, be.Err())
	})

	t.Run("one error", func(t *testing.T) {
		errFoo := errors.New("foo")
		errBar := errors.New("bar")
		var be BatchError
		be.Add(errFoo)
		assert.ErrorIs(t, be.Err(), errFoo)
		assert.NotErrorIs(t, be.Err(), errBar)
	})

	t.Run("two errors", func(t *testing.T) {
		errFoo := errors.New("foo")
		errBar := errors.New("bar")
		errBaz := errors.New("baz")
		var be BatchError
		be.Add(errFoo)
		be.Add(errBar)
		assert.ErrorIs(t, be.Err(), errFoo)
		assert.ErrorIs(t, be.Err(), errBar)
		assert.NotErrorIs(t, be.Err(), errBaz)
	})
}

func TestBatchError_Add(t *testing.T) {
	var be BatchError

	// Test adding nil errors
	be.Add(nil, nil)
	assert.False(t, be.NotNil(), "Expected BatchError to be empty after adding nil errors")

	// Test adding non-nil errors
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	be.Add(err1, err2)
	assert.True(t, be.NotNil(), "Expected BatchError to be non-empty after adding errors")

	// Test adding a mix of nil and non-nil errors
	err3 := errors.New("error 3")
	be.Add(nil, err3, nil)
	assert.True(t, be.NotNil(), "Expected BatchError to be non-empty after adding a mix of nil and non-nil errors")
}

func TestBatchError_Err(t *testing.T) {
	var be BatchError

	// Test Err() on empty BatchError
	assert.NoError(t, be.Err(), "Expected nil error for empty BatchError")

	// Test Err() with multiple errors
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")
	be.Add(err1, err2)

	combinedErr := be.Err()
	assert.Error(t, combinedErr, "Expected nil error for BatchError with multiple errors")

	// Check if the combined error contains both error messages
	errString := combinedErr.Error()
	assert.ErrorIsf(t, combinedErr, err1, "Combined error doesn't contain first error: %s", errString)
	assert.ErrorIsf(t, combinedErr, err2, "Combined error doesn't contain second error: %s", errString)
}

func TestBatchError_NotNil(t *testing.T) {
	var be BatchError

	// Test NotNil() on empty BatchError
	assert.NoError(t, be.Err(), "Expected nil error for empty BatchError")

	// Test NotNil() after adding an error
	be.Add(errors.New("test error"))
	assert.Error(t, be.Err(), "Expected non-nil error after adding an error")
}
