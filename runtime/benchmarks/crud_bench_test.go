package benchmarks

import (
	"testing"

	"github.com/goccy/go-json"
)

type benchItem struct {
	ID    int64   `db:"id,primary,auto"`
	Name  string  `db:"name"`
	Price float64 `db:"price"`
}

func BenchmarkCrudOperations(b *testing.B) {
	b.Run("marshal", func(b *testing.B) {
		item := benchItem{ID: 1, Name: "test", Price: 9.99}
		b.ReportAllocs()
		for b.Loop() {
			_, _ = json.Marshal(item)
		}
	})
	b.Run("unmarshal", func(b *testing.B) {
		data := []byte(`{"id":1,"name":"test","price":9.99}`)
		b.ReportAllocs()
		for b.Loop() {
			var item benchItem
			_ = json.Unmarshal(data, &item)
		}
	})
}
