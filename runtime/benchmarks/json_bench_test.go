package benchmarks

import (
	"encoding/json"
	"testing"
)

type benchProduct struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Price    float64 `json:"price"`
	Category string  `json:"category"`
	SKU      string  `json:"sku"`
	Active   bool    `json:"active"`
}

var benchData = benchProduct{
	ID: 42, Name: "Widget Pro", Price: 29.99,
	Category: "electronics", SKU: "WIDG-042", Active: true,
}

func BenchmarkJSONMarshal(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, _ = json.Marshal(benchData)
	}
}

func BenchmarkJSONUnmarshal(b *testing.B) {
	data, _ := json.Marshal(benchData)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var p benchProduct
		_ = json.Unmarshal(data, &p)
	}
}

func BenchmarkJSONMarshalParallel(b *testing.B) {
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = json.Marshal(benchData)
		}
	})
}
