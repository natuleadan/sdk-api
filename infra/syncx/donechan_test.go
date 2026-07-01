package syncx

import (
	"sync"
	"testing"
)

func TestDoneChanClose(t *testing.T) {
	doneChan := NewDoneChan()

	for range 5 {
		doneChan.Close()
	}
}

func TestDoneChanDone(t *testing.T) {
	var waitGroup sync.WaitGroup
	doneChan := NewDoneChan()

	waitGroup.Go(func() {
		<-doneChan.Done()
	})

	for range 5 {
		doneChan.Close()
	}

	waitGroup.Wait()
}
