package contextx

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshalContext(t *testing.T) {
	type Person struct {
		Name string `ctx:"name"`
		Age  int    `ctx:"age"`
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKey("name"), "kevin")
	ctx = context.WithValue(ctx, ctxKey("age"), 20)

	var person Person
	err := For(ctx, &person)

	assert.NoError(t, err)
	assert.Equal(t, "kevin", person.Name)
	assert.Equal(t, 20, person.Age)
}

func TestUnmarshalContextWithOptional(t *testing.T) {
	type Person struct {
		Name string `ctx:"name"`
		Age  int    `ctx:"age,optional"`
	}

	ctx := context.Background()
	ctx = context.WithValue(ctx, ctxKey("name"), "kevin")

	var person Person
	err := For(ctx, &person)

	assert.NoError(t, err)
	assert.Equal(t, "kevin", person.Name)
	assert.Equal(t, 0, person.Age)
}

func TestUnmarshalContextWithMissing(t *testing.T) {
	type Person struct {
		Name string `ctx:"name"`
		Age  int    `ctx:"age"`
	}
	type name string
	const PersonNameKey name = "name"

	ctx := context.Background()
	ctx = context.WithValue(ctx, PersonNameKey, "kevin")

	var person Person
	err := For(ctx, &person)

	assert.Error(t, err)
}
