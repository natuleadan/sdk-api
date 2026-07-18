package utils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const sleepInterval = time.Millisecond * 10

func TestElapsedTimer_Duration(t *testing.T) {
	timer := NewElapsedTimer()
	time.Sleep(sleepInterval)
	assert.GreaterOrEqual(t, timer.Duration(), sleepInterval)
}

func TestElapsedTimer_Elapsed(t *testing.T) {
	timer := NewElapsedTimer()
	time.Sleep(sleepInterval)
	duration, err := time.ParseDuration(timer.Elapsed())
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, duration, sleepInterval)
}

func TestElapsedTimer_ElapsedMs(t *testing.T) {
	timer := NewElapsedTimer()
	time.Sleep(sleepInterval)
	duration, err := time.ParseDuration(timer.ElapsedMs())
	assert.NoError(t, err)
	assert.GreaterOrEqual(t, duration, sleepInterval)
}

func TestCurrent(t *testing.T) {
	currentMillis := CurrentMillis()
	currentMicros := CurrentMicros()
	assert.Positive(t, currentMillis)
	assert.Positive(t, currentMicros)
	assert.LessOrEqual(t, currentMillis*1000, currentMicros)
}
