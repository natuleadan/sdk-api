package benchmarks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type BenchRecord struct {
	Timestamp time.Time `json:"timestamp"`
	Name      string    `json:"name"`
	NsPerOp   float64   `json:"ns_per_op"`
	AllocsOp  int64     `json:"allocs_per_op"`
	BytesOp   int64     `json:"bytes_per_op"`
}

func SaveBenchRecord(records []BenchRecord) error {
	dir := "bench-history"
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}

	name := filepath.Join(dir, time.Now().Format("2006-01-02-150405")+".json")
	data, _ := json.MarshalIndent(records, "", "  ")
	return os.WriteFile(name, data, 0o600)
}
