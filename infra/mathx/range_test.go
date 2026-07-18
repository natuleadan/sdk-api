package mathx

import "testing"

type atLeastCase[T Numerical] struct {
	val, lower, want T
}

type atMostCase[T Numerical] struct {
	val, upper, want T
}

type betweenCase[T Numerical] struct {
	val, lower, upper, want T
}

func testAtLeast[T Numerical](t *testing.T, cases []atLeastCase[T]) {
	t.Helper()
	for _, c := range cases {
		if got := AtLeast(c.val, c.lower); got != c.want {
			t.Errorf("AtLeast(%v, %v) = %v, want %v", c.val, c.lower, got, c.want)
		}
	}
}

func testAtMost[T Numerical](t *testing.T, cases []atMostCase[T]) {
	t.Helper()
	for _, c := range cases {
		if got := AtMost(c.val, c.upper); got != c.want {
			t.Errorf("AtMost(%v, %v) = %v, want %v", c.val, c.upper, got, c.want)
		}
	}
}

func testBetween[T Numerical](t *testing.T, cases []betweenCase[T]) {
	t.Helper()
	for _, c := range cases {
		if got := Between(c.val, c.lower, c.upper); got != c.want {
			t.Errorf("Between(%v, %v, %v) = %v, want %v", c.val, c.lower, c.upper, got, c.want)
		}
	}
}

func TestAtLeast(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[int]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("int8", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[int8]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("int16", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[int16]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("int32", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[int32]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("int64", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[int64]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("uint", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[uint]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("uint8", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[uint8]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("uint16", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[uint16]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("uint32", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[uint32]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("uint64", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[uint64]{
			{10, 5, 10}, {3, 5, 5}, {5, 5, 5},
		})
	})
	t.Run("float32", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[float32]{
			{10.0, 5.0, 10.0}, {3.0, 5.0, 5.0}, {5.0, 5.0, 5.0},
		})
	})
	t.Run("float64", func(t *testing.T) {
		testAtLeast(t, []atLeastCase[float64]{
			{10.0, 5.0, 10.0}, {3.0, 5.0, 5.0}, {5.0, 5.0, 5.0},
		})
	})
}

func TestAtMost(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		testAtMost(t, []atMostCase[int]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("int8", func(t *testing.T) {
		testAtMost(t, []atMostCase[int8]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("int16", func(t *testing.T) {
		testAtMost(t, []atMostCase[int16]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("int32", func(t *testing.T) {
		testAtMost(t, []atMostCase[int32]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("int64", func(t *testing.T) {
		testAtMost(t, []atMostCase[int64]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("uint", func(t *testing.T) {
		testAtMost(t, []atMostCase[uint]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("uint8", func(t *testing.T) {
		testAtMost(t, []atMostCase[uint8]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("uint16", func(t *testing.T) {
		testAtMost(t, []atMostCase[uint16]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("uint32", func(t *testing.T) {
		testAtMost(t, []atMostCase[uint32]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("uint64", func(t *testing.T) {
		testAtMost(t, []atMostCase[uint64]{
			{10, 5, 5}, {3, 5, 3}, {5, 5, 5},
		})
	})
	t.Run("float32", func(t *testing.T) {
		testAtMost(t, []atMostCase[float32]{
			{10.0, 5.0, 5.0}, {3.0, 5.0, 3.0}, {5.0, 5.0, 5.0},
		})
	})
	t.Run("float64", func(t *testing.T) {
		testAtMost(t, []atMostCase[float64]{
			{10.0, 5.0, 5.0}, {3.0, 5.0, 3.0}, {5.0, 5.0, 5.0},
		})
	})
}

func TestBetween(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		testBetween(t, []betweenCase[int]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("int8", func(t *testing.T) {
		testBetween(t, []betweenCase[int8]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("int16", func(t *testing.T) {
		testBetween(t, []betweenCase[int16]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("int32", func(t *testing.T) {
		testBetween(t, []betweenCase[int32]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("int64", func(t *testing.T) {
		testBetween(t, []betweenCase[int64]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("uint", func(t *testing.T) {
		testBetween(t, []betweenCase[uint]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("uint8", func(t *testing.T) {
		testBetween(t, []betweenCase[uint8]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("uint16", func(t *testing.T) {
		testBetween(t, []betweenCase[uint16]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("uint32", func(t *testing.T) {
		testBetween(t, []betweenCase[uint32]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("uint64", func(t *testing.T) {
		testBetween(t, []betweenCase[uint64]{
			{10, 5, 15, 10}, {3, 5, 15, 5}, {20, 5, 15, 15}, {5, 5, 15, 5}, {15, 5, 15, 15},
		})
	})
	t.Run("float32", func(t *testing.T) {
		testBetween(t, []betweenCase[float32]{
			{10.0, 5.0, 15.0, 10.0},
			{3.0, 5.0, 15.0, 5.0},
			{20.0, 5.0, 15.0, 15.0},
			{5.0, 5.0, 15.0, 5.0},
			{15.0, 5.0, 15.0, 15.0},
		})
	})
	t.Run("float64", func(t *testing.T) {
		testBetween(t, []betweenCase[float64]{
			{10.0, 5.0, 15.0, 10.0},
			{3.0, 5.0, 15.0, 5.0},
			{20.0, 5.0, 15.0, 15.0},
			{5.0, 5.0, 15.0, 5.0},
			{15.0, 5.0, 15.0, 15.0},
		})
	})
}
