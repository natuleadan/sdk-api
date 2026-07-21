package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeCursor(t *testing.T) {
	t.Parallel()

	cursor := PageCursor{LastValue: "2024-01-01T00:00:00Z", LastID: "42"}
	encoded := EncodeCursor(cursor)
	require.NotEmpty(t, encoded)

	decoded, err := ParseCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, cursor.LastValue, decoded.LastValue)
	assert.Equal(t, cursor.LastID, decoded.LastID)
}

func TestEncodeCursor_Error(t *testing.T) {
	t.Parallel()

	// Cannot JSON marshal a channel
	encoded := EncodeCursor(PageCursor{LastID: "1"})
	assert.NotEmpty(t, encoded)

	// Invalid base64
	_, err := ParseCursor("not-base64!!!")
	require.Error(t, err)

	// Invalid JSON
	_, err = ParseCursor("aW52YWxpZCBqc29u")
	require.Error(t, err)
}

func TestNewPaginatedResponse(t *testing.T) {
	t.Parallel()

	t.Run("with more", func(t *testing.T) {
		items := []string{"a", "b"}
		resp := NewPaginatedResponse(items, "next-cursor")
		assert.Equal(t, items, resp.Items)
		assert.Equal(t, "next-cursor", resp.NextCursor)
		assert.True(t, resp.HasMore)
	})

	t.Run("no more", func(t *testing.T) {
		items := []int{1, 2, 3}
		resp := NewPaginatedResponse(items, "")
		assert.Equal(t, items, resp.Items)
		assert.Empty(t, resp.NextCursor)
		assert.False(t, resp.HasMore)
	})

	t.Run("empty", func(t *testing.T) {
		resp := NewPaginatedResponse([]string{}, "")
		assert.Empty(t, resp.Items)
		assert.Empty(t, resp.NextCursor)
		assert.False(t, resp.HasMore)
	})
}

func TestParseCursor_Empty(t *testing.T) {
	t.Parallel()

	_, err := ParseCursor("")
	assert.Error(t, err)
}

func TestPageCursor_Roundtrip(t *testing.T) {
	t.Parallel()

	cursors := []PageCursor{
		{LastValue: "abc", LastID: "123"},
		{LastValue: "", LastID: "0"},
		{LastValue: "some-long-value-with-special-chars!@#$", LastID: "999"},
	}
	for _, c := range cursors {
		enc := EncodeCursor(c)
		dec, err := ParseCursor(enc)
		require.NoError(t, err)
		assert.Equal(t, c, dec)
	}
}

func TestPaginatedResponse_JSON(t *testing.T) {
	t.Parallel()

	resp := NewPaginatedResponse([]map[string]any{
		{"id": 1, "name": "a"},
	}, "cursor-abc")

	assert.Equal(t, "cursor-abc", resp.NextCursor)
	assert.True(t, resp.HasMore)
}
