package load

import (
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/natuleadan/sdk-api/infra/collection"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/mathx"
	"github.com/natuleadan/sdk-api/infra/stat"
	"github.com/natuleadan/sdk-api/infra/syncx"
	"github.com/natuleadan/sdk-api/infra/timex"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	buckets        = 10
	bucketDuration = time.Millisecond * 50
	windowFactor   = 0.01
)

func init() {
	stat.SetReporter(nil)
}

func TestAdaptiveShedder(t *testing.T) {
	DisableLog()
	shedder := NewAdaptiveShedder(WithWindow(bucketDuration), WithBuckets(buckets), WithCpuThreshold(100))
	var wg sync.WaitGroup
	var drop int64
	proba := mathx.NewProba()
	for range 100 {
		wg.Go(func() {
			for range 30 {
				promise, err := shedder.Allow()
				if err != nil {
					atomic.AddInt64(&drop, 1)
				} else {
					count := rand.Intn(5)
					time.Sleep(time.Millisecond * time.Duration(count))
					if proba.TrueOnProba(0.01) {
						promise.Fail()
					} else {
						promise.Pass()
					}
				}
			}
		})
	}
	wg.Wait()
}

func TestAdaptiveShedderMaxPass(t *testing.T) {
	passCounter := newRollingWindow()
	for i := 1; i <= 10; i++ {
		passCounter.Add(int64(i * 100))
		time.Sleep(bucketDuration)
	}
	shedder := &adaptiveShedder{
		passCounter:     passCounter,
		droppedRecently: syncx.NewAtomicBool(),
	}
	assert.Equal(t, int64(1000), shedder.maxPass())

	// default max pass is equal to 1.
	passCounter = newRollingWindow()
	shedder = &adaptiveShedder{
		passCounter:     passCounter,
		droppedRecently: syncx.NewAtomicBool(),
	}
	assert.Equal(t, int64(1), shedder.maxPass())
}

func TestAdaptiveShedderMinRt(t *testing.T) {
	rtCounter := newRollingWindow()
	for i := range 10 {
		if i > 0 {
			time.Sleep(bucketDuration)
		}
		for j := i*10 + 1; j <= i*10+10; j++ {
			rtCounter.Add(int64(j))
		}
	}
	shedder := &adaptiveShedder{
		rtCounter: rtCounter,
	}
	assert.InDelta(t, float64(6), shedder.minRt(), 0.01)

	// default max min rt is equal to maxFloat64.
	rtCounter = newRollingWindow()
	shedder = &adaptiveShedder{
		rtCounter:       rtCounter,
		droppedRecently: syncx.NewAtomicBool(),
	}
	assert.Equal(t, defaultMinRt, shedder.minRt())
}

func TestAdaptiveShedderMaxFlight(t *testing.T) {
	passCounter := newRollingWindow()
	rtCounter := newRollingWindow()
	for i := range 10 {
		if i > 0 {
			time.Sleep(bucketDuration)
		}
		passCounter.Add(int64((i + 1) * 100))
		for j := i*10 + 1; j <= i*10+10; j++ {
			rtCounter.Add(int64(j))
		}
	}
	shedder := &adaptiveShedder{
		passCounter:     passCounter,
		rtCounter:       rtCounter,
		windowScale:     windowFactor,
		droppedRecently: syncx.NewAtomicBool(),
	}
	assert.Equal(t, float64(54), shedder.maxFlight())
}

func TestAdaptiveShedderShouldDrop(t *testing.T) {
	logx.Disable()
	passCounter := newRollingWindow()
	rtCounter := newRollingWindow()
	for i := range 10 {
		if i > 0 {
			time.Sleep(bucketDuration)
		}
		passCounter.Add(int64((i + 1) * 100))
		for j := i*10 + 1; j <= i*10+10; j++ {
			rtCounter.Add(int64(j))
		}
	}
	shedder := &adaptiveShedder{
		passCounter:     passCounter,
		rtCounter:       rtCounter,
		windowScale:     windowFactor,
		overloadTime:    syncx.NewAtomicDuration(),
		droppedRecently: syncx.NewAtomicBool(),
	}
	// cpu >=  800, inflight < maxPass
	systemOverloadChecker = func(int64) bool {
		return true
	}
	shedder.avgFlying = 50
	assert.False(t, shedder.shouldDrop())

	// cpu >=  800, inflight > maxPass
	shedder.avgFlying = 80
	// because of the overloadFactor, so we need to make sure maxFlight is greater than flying
	shedder.flying = int64(shedder.maxFlight()*shedder.overloadFactor()) - 5
	assert.False(t, shedder.shouldDrop())

	// cpu >=  800, inflight > maxPass
	shedder.avgFlying = 80
	shedder.flying = 80
	assert.True(t, shedder.shouldDrop())

	// cpu < 800, inflight > maxPass
	systemOverloadChecker = func(int64) bool {
		return false
	}
	shedder.avgFlying = 80
	assert.False(t, shedder.shouldDrop())

	// cpu >=  800, inflight < maxPass
	systemOverloadChecker = func(int64) bool {
		return true
	}
	shedder.avgFlying = 80
	shedder.flying = 80
	_, err := shedder.Allow()
	require.Error(t, err)
}

func TestAdaptiveShedderStillHot(t *testing.T) {
	logx.Disable()
	passCounter := newRollingWindow()
	rtCounter := newRollingWindow()
	for i := range 10 {
		if i > 0 {
			time.Sleep(bucketDuration)
		}
		passCounter.Add(int64((i + 1) * 100))
		for j := i*10 + 1; j <= i*10+10; j++ {
			rtCounter.Add(int64(j))
		}
	}
	shedder := &adaptiveShedder{
		passCounter:     passCounter,
		rtCounter:       rtCounter,
		windowScale:     windowFactor,
		overloadTime:    syncx.NewAtomicDuration(),
		droppedRecently: syncx.ForAtomicBool(true),
	}
	assert.False(t, shedder.stillHot())
	shedder.overloadTime.Set(-coolOffDuration * 2)
	assert.False(t, shedder.stillHot())
	shedder.droppedRecently.Set(true)
	shedder.overloadTime.Set(timex.Now())
	assert.True(t, shedder.stillHot())
}

func BenchmarkAdaptiveShedder_Allow(b *testing.B) {
	logx.Disable()

	bench := func(b *testing.B) {
		shedder := NewAdaptiveShedder()
		proba := mathx.NewProba()
		for range 6000 {
			p, err := shedder.Allow()
			if err == nil {
				time.Sleep(time.Millisecond)
				if proba.TrueOnProba(0.01) {
					p.Fail()
				} else {
					p.Pass()
				}
			}
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			p, err := shedder.Allow()
			if err == nil {
				p.Pass()
			}
		}
	}

	systemOverloadChecker = func(int64) bool {
		return true
	}
	b.Run("high load", bench)
	systemOverloadChecker = func(int64) bool {
		return false
	}
	b.Run("low load", bench)
}

func BenchmarkMaxFlight(b *testing.B) {
	passCounter := newRollingWindow()
	rtCounter := newRollingWindow()
	for i := range 10 {
		if i > 0 {
			time.Sleep(bucketDuration)
		}
		passCounter.Add(int64((i + 1) * 100))
		for j := i*10 + 1; j <= i*10+10; j++ {
			rtCounter.Add(int64(j))
		}
	}
	shedder := &adaptiveShedder{
		passCounter:     passCounter,
		rtCounter:       rtCounter,
		windowScale:     windowFactor,
		droppedRecently: syncx.NewAtomicBool(),
	}

	for i := 0; i < b.N; i++ {
		_ = shedder.maxFlight()
	}
}

func newRollingWindow() *collection.RollingWindow[int64, *collection.Bucket[int64]] {
	return collection.NewRollingWindow[int64, *collection.Bucket[int64]](func() *collection.Bucket[int64] {
		return new(collection.Bucket[int64])
	}, buckets, bucketDuration, collection.IgnoreCurrentBucket[int64, *collection.Bucket[int64]]())
}
