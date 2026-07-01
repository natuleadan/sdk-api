package sysx

import (
	"fmt"
	"os"

	"go.uber.org/automaxprocs/maxprocs"
)

// Automatically set GOMAXPROCS to match Linux container CPU quota.
func init() {
	if _, err := maxprocs.Set(maxprocs.Logger(nil)); err != nil {
		fmt.Fprintf(os.Stderr, "sysx: automaxprocs error: %v\n", err)
	}
}
