package runtime

import (
	"sync"
	"time"
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
	ID        string    `json:"id"`
	Status    JobStatus `json:"status"`
	Result    any       `json:"result,omitempty"`
	Error     string    `json:"error,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// JobStore persists and retrieves job state.
type JobStore interface {
	Create(id string) *JobState
	Get(id string) (*JobState, bool)
	Update(id string, status JobStatus, result any, errMsg string)
	Delete(id string)
}

// memoryJobStore is an in-memory implementation of JobStore for testing
// and single-instance use without NATS.
type memoryJobStore struct {
	mu   sync.RWMutex
	jobs map[string]*JobState
}

func newMemoryJobStore() *memoryJobStore {
	return &memoryJobStore{jobs: make(map[string]*JobState)}
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
	}
}

func (s *memoryJobStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.jobs, id)
}
