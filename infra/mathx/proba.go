package mathx

import (
	"crypto/rand"
	"encoding/binary"
	"sync"
)

// A Proba is used to test if true on given probability.
type Proba struct {
	lock sync.Mutex
	buf  [8]byte
}

// NewProba returns a Proba.
func NewProba() *Proba {
	return &Proba{}
}

// TrueOnProba checks if true on given probability.
func (p *Proba) TrueOnProba(proba float64) bool {
	p.lock.Lock()
	defer p.lock.Unlock()
	if _, err := rand.Read(p.buf[:]); err != nil {
		return false
	}
	r := float64(binary.LittleEndian.Uint64(p.buf[:])) / float64(1<<64)
	return r < proba
}
