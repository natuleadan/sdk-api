package runtime

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/proc.init.1.func1"),
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/stat.init.0.func1"),
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/stat.init.1.func1"),
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/collection.(*TimingWheel).run"),
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.updateServerDate.func1"),
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.(*workerPool).Start.func2"),
		goleak.IgnoreAnyFunction("github.com/valyala/fasthttp.(*workerPool).getCh"),
		goleak.IgnoreAnyFunction("net/http.(*persistConn).writeLoop"),
		goleak.IgnoreAnyFunction("net/http.(*persistConn).readLoop"),
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/server/middleware.(*rateLimiterStore).gcLoop"),
		goleak.IgnoreAnyFunction("internal/poll.runtime_pollWait"),
	)
}
