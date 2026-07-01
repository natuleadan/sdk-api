package mr

import (
	"fmt"
	"math/rand"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"
)

func FuzzMapReduce(f *testing.F) {
	rand.NewSource(time.Now().UnixNano())

	f.Add(int64(10), runtime.NumCPU())
	f.Fuzz(func(t *testing.T, n int64, workers int) {
		n = n%5000 + 5000
		genPanic := rand.Intn(100) == 0
		mapperPanic := rand.Intn(100) == 0
		reducerPanic := rand.Intn(100) == 0
		genIdx := rand.Int63n(n)
		mapperIdx := rand.Int63n(n)
		reducerIdx := rand.Int63n(n)
		squareSum := (n - 1) * n * (2*n - 1) / 6

		fn := func() (int64, error) {
			defer goleak.VerifyNone(t, goleak.IgnoreCurrent())

			return MapReduce(func(source chan<- int64) {
				for i := int64(0); i < n; i++ {
					source <- i
					if genPanic && i == genIdx {
						panic("foo")
					}
				}
			}, func(v int64, writer Writer[int64], cancel func(error)) {
				if mapperPanic && v == mapperIdx {
					panic("bar")
				}
				writer.Write(v * v)
			}, func(pipe <-chan int64, writer Writer[int64], cancel func(error)) {
				var idx int64
				var total int64
				for v := range pipe {
					if reducerPanic && idx == reducerIdx {
						panic("baz")
					}
					total += v
					idx++
				}
				writer.Write(total)
			}, WithWorkers(workers%50+runtime.NumCPU()))
		}

		if genPanic || mapperPanic || reducerPanic {
			var buf strings.Builder
			fmt.Fprintf(&buf, "n: %d", n)
			fmt.Fprintf(&buf, ", genPanic: %t", genPanic)
			fmt.Fprintf(&buf, ", mapperPanic: %t", mapperPanic)
			fmt.Fprintf(&buf, ", reducerPanic: %t", reducerPanic)
			fmt.Fprintf(&buf, ", genIdx: %d", genIdx)
			fmt.Fprintf(&buf, ", mapperIdx: %d", mapperIdx)
			fmt.Fprintf(&buf, ", reducerIdx: %d", reducerIdx)
			assert.Panicsf(t, func() { fn() }, buf.String())
		} else {
			val, err := fn()
			assert.Nil(t, err)
			assert.Equal(t, squareSum, val)
		}
	})
}
