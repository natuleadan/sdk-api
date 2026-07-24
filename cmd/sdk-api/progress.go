package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// ProgressIndicator displays step-by-step progress for long operations.
type ProgressIndicator struct {
	mu      sync.Mutex
	steps   []string
	current int
	writer  io.Writer
	started time.Time
	failed  bool
}

// NewProgress creates a progress indicator for the given step descriptions.
// Output is written to os.Stderr by default.
func NewProgress(steps []string) *ProgressIndicator {
	return &ProgressIndicator{
		steps:   steps,
		writer:  os.Stderr,
		started: time.Now(),
	}
}

// WithWriter configures a custom output writer (for testing).
func (p *ProgressIndicator) WithWriter(w io.Writer) *ProgressIndicator {
	p.writer = w
	return p
}

// Step advances to the next step and displays its name.
func (p *ProgressIndicator) Step(name string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.current++
	elapsed := time.Since(p.started).Round(time.Millisecond)
	if _, err := fmt.Fprintf(p.writer, "  %d/%d  %s  (%s)\n", p.current, len(p.steps), name, elapsed); err != nil {
		p.failed = true
	}
}

// Fail marks the current step as failed.
func (p *ProgressIndicator) Fail(msg string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.failed = true
	elapsed := time.Since(p.started).Round(time.Millisecond)
	if _, err := fmt.Fprintf(p.writer, "  ✗  %s failed: %s  (%s)\n", p.steps[p.current-1], msg, elapsed); err != nil {
		return
	}
}

// Done marks all steps as complete.
func (p *ProgressIndicator) Done() {
	p.mu.Lock()
	defer p.mu.Unlock()
	elapsed := time.Since(p.started).Round(time.Millisecond)
	if _, err := fmt.Fprintf(p.writer, "  ✓  %d/%d complete in %s\n", p.current, len(p.steps), elapsed); err != nil {
		return
	}
}
