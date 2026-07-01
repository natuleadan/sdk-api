//go:build linux

package stat

import (
	"flag"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/natuleadan/sdk-api/infra/executors"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/proc"
	"github.com/natuleadan/sdk-api/infra/sysx"
)

const (
	clusterNameKey = "CLUSTER_NAME"
	testEnv        = "test.v"
)

var (
	reporter     = logx.Alert
	lock         sync.RWMutex
	lessExecutor = executors.NewLessExecutor(time.Minute * 5)
	dropped      int32
	clusterName  = proc.Env(clusterNameKey)
)

func init() {
	if flag.Lookup(testEnv) != nil {
		SetReporter(nil)
	}
}

// Report reports given message.
func Report(msg string) {
	lock.RLock()
	fn := reporter
	lock.RUnlock()

	if fn != nil {
		reported := lessExecutor.DoOrDiscard(func() {
			var builder strings.Builder
			fmt.Fprintln(&builder, time.Now().Format(time.DateTime))
			if len(clusterName) > 0 {
				fmt.Fprintf(&builder, "cluster: %s\n", clusterName)
			}
			fmt.Fprintf(&builder, "host: %s\n", sysx.Hostname())
			dp := atomic.SwapInt32(&dropped, 0)
			if dp > 0 {
				fmt.Fprintf(&builder, "dropped: %d\n", dp)
			}
			builder.WriteString(strings.TrimSpace(msg))
			fn(builder.String())
		})
		if !reported {
			atomic.AddInt32(&dropped, 1)
		}
	}
}

// SetReporter sets the given reporter.
func SetReporter(fn func(string)) {
	lock.Lock()
	defer lock.Unlock()
	reporter = fn
}
