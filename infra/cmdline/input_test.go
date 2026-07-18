package cmdline

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/natuleadan/sdk-api/infra/iox"
	"github.com/natuleadan/sdk-api/infra/lang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnterToContinue(t *testing.T) {
	restore, err := iox.RedirectInOut()
	require.NoError(t, err)
	defer restore()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fmt.Println()
	}()
	go func() {
		defer wg.Done()
		EnterToContinue()
	}()

	wait := make(chan lang.PlaceholderType)
	go func() {
		wg.Wait()
		close(wait)
	}()

	select {
	case <-time.After(time.Second):
		t.Error("timeout")
	case <-wait:
	}
}

func TestReadLine(t *testing.T) {
	r, w, err := os.Pipe()
	require.NoError(t, err)
	ow := os.Stdout
	os.Stdout = w
	or := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = or
		os.Stdout = ow
	}()

	const message = "hello"
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		fmt.Println(message)
	}()
	go func() {
		defer wg.Done()
		input := ReadLine("")
		assert.Equal(t, message, input)
	}()

	wait := make(chan lang.PlaceholderType)
	go func() {
		wg.Wait()
		close(wait)
	}()

	select {
	case <-time.After(time.Second):
		t.Error("timeout")
	case <-wait:
	}
}
