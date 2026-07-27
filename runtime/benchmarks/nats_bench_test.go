package benchmarks

import (
	"testing"
)

func BenchmarkNATSPublish(b *testing.B) {
	b.Skip("requires NATS connection")
}

func BenchmarkNATSSubscribe(b *testing.B) {
	b.Skip("requires NATS connection")
}
