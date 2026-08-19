package events

import (
	"testing"

	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/proc.init.1.func1"),
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/stat.init.0.func1"),
		goleak.IgnoreAnyFunction("github.com/natuleadan/sdk-api/infra/collection.(*TimingWheel).run"),
		goleak.IgnoreAnyFunction("github.com/nats-io/nats%2ego.(*Conn).drainConnection"),
		goleak.IgnoreAnyFunction("github.com/nats-io/nats%2ego.(*Subscription).Fetch"),
	)
}
