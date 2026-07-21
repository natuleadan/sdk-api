package runtime

import (
	"sync"

	"github.com/natuleadan/sdk-api/infra/syncx"
)

var (
	bulkheadMu   sync.RWMutex
	bulkheadSems = make(map[string]*syncx.Limit)
)

func BulkheadGet(name string) *syncx.Limit {
	bulkheadMu.RLock()
	sem, ok := bulkheadSems[name]
	bulkheadMu.RUnlock()
	if ok {
		return sem
	}
	return nil
}

func BulkheadRegister(name string, limit int) {
	bulkheadMu.Lock()
	if _, ok := bulkheadSems[name]; !ok {
		sem := syncx.NewLimit(limit)
		bulkheadSems[name] = &sem
	}
	bulkheadMu.Unlock()
}
