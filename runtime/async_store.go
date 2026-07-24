package runtime

import (
	"context"
	"sync"
	"time"

	"github.com/natuleadan/sdk-api/infra/logx"
)

// JobStatus represents the processing state of a job.
type JobStatus string

const (
	JobPending    JobStatus = "pending"
	JobProcessing JobStatus = "processing"
	JobCompleted  JobStatus = "completed"
	JobFailed     JobStatus = "failed"
)

// JobState holds the state and result of an async job.
type JobState struct {
	ID                 string     `json:"id"`
	Status             JobStatus  `json:"status"`
	Result             any        `json:"result,omitempty"`
	Error              string     `json:"error,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	RetryCount         int        `json:"retry_count,omitempty"`
	MaxRetries         int        `json:"max_retries,omitempty"`
	ProcessingDeadline *time.Time `json:"processing_deadline,omitempty"`
	CallbackURL        string     `json:"callback_url,omitempty"`
}

// JobStore persists and retrieves job state.
type JobStore interface {
	Create(id string) *JobState
	Get(id string) (*JobState, bool)
	Update(id string, status JobStatus, result any, errMsg string)
	Delete(id string)
	// List returns all jobs (best-effort, limited).
	List() ([]*JobState, error)
	// ReapStale resets jobs stuck in "processing" for longer than the deadline.
	// Returns the number of jobs reaped.
	ReapStale(ctx context.Context, timeout time.Duration, maxRetries int) (int, error)
	// Cleanup removes completed and failed jobs older than ttl.
	// Returns the number of jobs cleaned.
	Cleanup(ctx context.Context, ttl time.Duration) (int, error)
}

// Reaper periodically calls ReapStale on a JobStore to recover stuck jobs.
type Reaper struct {
	store      JobStore
	timeout    time.Duration
	interval   time.Duration
	maxRetry   int
	cleanupTTL time.Duration
	ctx        context.Context
	cancel     context.CancelFunc
}

// NewReaper creates a Reaper that runs every interval.
func NewReaper(store JobStore, timeout, interval time.Duration, maxRetries int) *Reaper {
	ctx, cancel := context.WithCancel(context.Background())
	return &Reaper{
		store:    store,
		timeout:  timeout,
		interval: interval,
		maxRetry: maxRetries,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start begins the reap loop in a background goroutine.
func (r *Reaper) Start() {
	go func() {
		ticker := time.NewTicker(r.interval)
		defer ticker.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				n, err := r.store.ReapStale(r.ctx, r.timeout, r.maxRetry)
				if err != nil {
					logx.Errorf("reaper: %v", err)
				} else if n > 0 {
					logx.Infof("reaper: %d stuck jobs reaped", n)
				}
				if r.cleanupTTL > 0 {
					nc, cerr := r.store.Cleanup(r.ctx, r.cleanupTTL)
					if cerr != nil {
						logx.Errorf("reaper cleanup: %v", cerr)
					} else if nc > 0 {
						logx.Infof("reaper: %d old jobs cleaned", nc)
					}
				}
			}
		}
	}()
}

// Stop terminates the reap loop.
func (r *Reaper) Stop() {
	r.cancel()
}

// memoryJobStore is an in-memory implementation of JobStore for testing
// and single-instance use without NATS.
type memoryJobStore struct {
	mu                sync.RWMutex
	jobs              map[string]*JobState
	processingTimeout time.Duration
}

func newMemoryJobStore() *memoryJobStore {
	return &memoryJobStore{jobs: make(map[string]*JobState), processingTimeout: 5 * time.Minute}
}

func (s *memoryJobStore) Create(id string) *JobState {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	js := &JobState{ID: id, Status: JobPending, CreatedAt: now, UpdatedAt: now}
	s.jobs[id] = js
	return js
}

func (s *memoryJobStore) Get(id string) (*JobState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	js, ok := s.jobs[id]
	return js, ok
}

func (s *memoryJobStore) Update(id string, status JobStatus, result any, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if js, ok := s.jobs[id]; ok {
		js.Status = status
		js.Result = result
		js.Error = errMsg
		js.UpdatedAt = time.Now()
		if status == JobProcessing {
			dl := time.Now().Add(s.processingTimeout)
			js.ProcessingDeadline = &dl
		} else {
			js.ProcessingDeadline = nil
		}
	}
}

func (s *memoryJobStore) List() ([]*JobState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*JobState, 0, len(s.jobs))
	for _, js := range s.jobs {
		result = append(result, js)
	}
	return result, nil
}

func (s *memoryJobStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}

func (s *memoryJobStore) ReapStale(_ context.Context, timeout time.Duration, maxRetries int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	var count int
	for _, js := range s.jobs {
		if js.Status != JobProcessing {
			continue
		}
		if js.ProcessingDeadline == nil || now.Before(*js.ProcessingDeadline) {
			continue
		}
		if js.RetryCount >= maxRetries {
			js.Status = JobFailed
			js.Error = "max retries exceeded"
			js.ProcessingDeadline = nil
			js.UpdatedAt = now
			count++
			continue
		}
		js.Status = JobPending
		js.RetryCount++
		js.ProcessingDeadline = nil
		js.UpdatedAt = now
		count++
	}
	return count, nil
}

func (s *memoryJobStore) Cleanup(_ context.Context, ttl time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().Add(-ttl)
	var count int
	for id, js := range s.jobs {
		if js.Status == JobCompleted || js.Status == JobFailed {
			if js.UpdatedAt.Before(cutoff) {
				delete(s.jobs, id)
				count++
			}
		}
	}
	return count, nil
}
