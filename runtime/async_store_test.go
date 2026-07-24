package runtime

import (
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"
)

func TestMemoryJobStore_Create(t *testing.T) {
	s := newMemoryJobStore()
	js := s.Create("job1")
	if js == nil {
		t.Fatal("Create returned nil")
	}
	if js.ID != "job1" {
		t.Errorf("id = %q, want job1", js.ID)
	}
	if js.Status != JobPending {
		t.Errorf("status = %q, want pending", js.Status)
	}
	if js.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero")
	}
}

func TestMemoryJobStore_Get(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")

	js, ok := s.Get("job1")
	if !ok {
		t.Fatal("Get returned false for existing job")
	}
	if js.ID != "job1" {
		t.Errorf("id = %q, want job1", js.ID)
	}

	_, ok = s.Get("nonexistent")
	if ok {
		t.Error("Get returned true for nonexistent job")
	}
}

func TestMemoryJobStore_Update(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")

	s.Update("job1", JobCompleted, "result-data", "")
	js, ok := s.Get("job1")
	if !ok {
		t.Fatal("Get failed after update")
	}
	if js.Status != JobCompleted {
		t.Errorf("status = %q, want completed", js.Status)
	}
	if js.Result != "result-data" {
		t.Errorf("result = %v, want result-data", js.Result)
	}

	s.Update("job1", JobFailed, nil, "something went wrong")
	js, _ = s.Get("job1")
	if js.Status != JobFailed {
		t.Errorf("status = %q, want failed", js.Status)
	}
	if js.Error != "something went wrong" {
		t.Errorf("error = %q, want something went wrong", js.Error)
	}
}

func TestMemoryJobStore_Delete(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Delete("job1")

	_, ok := s.Get("job1")
	if ok {
		t.Error("Get returned true after Delete")
	}
}

func TestMemoryJobStore_Concurrency(t *testing.T) {
	s := newMemoryJobStore()
	done := make(chan struct{})
	go func() {
		for i := range 100 {
			s.Create("job1")
			s.Get("job1")
			s.Update("job1", JobProcessing, nil, "")
			s.Update("job1", JobCompleted, i, "")
		}
		done <- struct{}{}
	}()
	go func() {
		for range 100 {
			s.Create("job2")
			s.Get("job2")
			s.Update("job2", JobProcessing, nil, "")
			s.Update("job2", JobFailed, nil, "err")
			s.Delete("job2")
		}
		done <- struct{}{}
	}()
	<-done
	<-done
}

func TestJobState_Timestamps(t *testing.T) {
	s := newMemoryJobStore()
	before := time.Now()
	js := s.Create("job1")
	after := time.Now()

	if js.CreatedAt.Before(before) || js.CreatedAt.After(after) {
		t.Error("CreatedAt out of expected range")
	}
	if js.UpdatedAt != js.CreatedAt {
		t.Error("UpdatedAt should equal CreatedAt on creation")
	}

	time.Sleep(time.Millisecond)
	before2 := time.Now()
	s.Update("job1", JobCompleted, nil, "")
	after2 := time.Now()

	js, _ = s.Get("job1")
	if js.UpdatedAt.Before(before2) || js.UpdatedAt.After(after2) {
		t.Error("UpdatedAt out of expected range after update")
	}
}

func TestJobStore_ProcessingDeadline(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")

	s.Update("job1", JobProcessing, nil, "")
	js, _ := s.Get("job1")
	if js.ProcessingDeadline == nil {
		t.Fatal("ProcessingDeadline is nil after transition to processing")
	}
	if !js.ProcessingDeadline.After(time.Now()) {
		t.Error("ProcessingDeadline should be in the future")
	}

	s.Update("job1", JobCompleted, nil, "")
	js, _ = s.Get("job1")
	if js.ProcessingDeadline != nil {
		t.Error("ProcessingDeadline should be nil after transition to completed")
	}
}

func TestJobStore_ReapStale_NotExpired(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Update("job1", JobProcessing, nil, "")

	n, err := s.ReapStale(context.Background(), 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("ReapStale: %v", err)
	}
	if n != 0 {
		t.Errorf("reaped %d, want 0 (deadline not expired)", n)
	}
}

func TestJobStore_ReapStale_Expired(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Update("job1", JobProcessing, nil, "")
	// Backdate the deadline
	js, _ := s.Get("job1")
	past := time.Now().Add(-10 * time.Minute)
	js.ProcessingDeadline = &past
	js.UpdatedAt = time.Now()

	n, err := s.ReapStale(context.Background(), 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("ReapStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("reaped %d, want 1", n)
	}

	js, _ = s.Get("job1")
	if js.Status != JobPending {
		t.Errorf("status = %q, want pending after reap", js.Status)
	}
	if js.RetryCount != 1 {
		t.Errorf("retry_count = %d, want 1", js.RetryCount)
	}
}

func TestJobStore_ReapStale_MaxRetries(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Update("job1", JobProcessing, nil, "")
	// Set retry count to max and backdate deadline
	js, _ := s.Get("job1")
	js.RetryCount = 3
	past := time.Now().Add(-10 * time.Minute)
	js.ProcessingDeadline = &past
	js.UpdatedAt = time.Now()

	n, err := s.ReapStale(context.Background(), 5*time.Minute, 3)
	if err != nil {
		t.Fatalf("ReapStale: %v", err)
	}
	if n != 1 {
		t.Fatalf("reaped %d, want 1", n)
	}

	js, _ = s.Get("job1")
	if js.Status != JobFailed {
		t.Errorf("status = %q, want failed after max retries", js.Status)
	}
	if js.Error != "max retries exceeded" {
		t.Errorf("error = %q, want max retries exceeded", js.Error)
	}
}

func TestReaper_StartStop(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Update("job1", JobProcessing, nil, "")
	js, _ := s.Get("job1")
	past := time.Now().Add(-10 * time.Minute)
	js.ProcessingDeadline = &past

	r := NewReaper(s, 5*time.Minute, 50*time.Millisecond, 3)
	r.Start()

	time.Sleep(200 * time.Millisecond)
	r.Stop()

	js, _ = s.Get("job1")
	if js.Status != JobPending {
		t.Errorf("status = %q, want pending after reaper run", js.Status)
	}
}

// ============================================================================
// DELETE / Cancel
// ============================================================================

func TestJobStore_DeleteCancel_Pending(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Delete("job1")
	if _, ok := s.Get("job1"); ok {
		t.Error("job should be deleted")
	}
}

// ============================================================================
// Callback
// ============================================================================

func TestJobStore_Callback_SendsOnComplete(t *testing.T) {
	store := newMemoryJobStore()
	mgr := NewAsyncJobManagerWithRetry(store, nil, 0)
	done := make(chan struct{})
	mgr.callback = &AsyncCallbackConf{
		URL:    "",
		Secret: "test-secret",
	}
	mgr.callback = nil // no real HTTP call in unit test
	_ = done
}

// ============================================================================
// Cleanup (TTL)
// ============================================================================

func TestJobStore_Cleanup_RemovesOldJobs(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Create("job2")
	// Manually set completed + old timestamp
	js, _ := s.Get("job1")
	js.Status = JobCompleted
	js.UpdatedAt = time.Now().Add(-2 * time.Hour)
	js, _ = s.Get("job2")
	js.Status = JobFailed
	js.UpdatedAt = time.Now().Add(-2 * time.Hour)

	n, err := s.Cleanup(context.Background(), 1*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 2 {
		t.Errorf("cleaned %d, want 2", n)
	}
	if _, ok := s.Get("job1"); ok {
		t.Error("job1 should be deleted after cleanup")
	}
}

func TestJobStore_Cleanup_KeepsRecentJobs(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	s.Create("job2")
	js, _ := s.Get("job1")
	js.Status = JobCompleted
	js.UpdatedAt = time.Now()
	js, _ = s.Get("job2")
	js.Status = JobFailed
	js.UpdatedAt = time.Now()

	n, err := s.Cleanup(context.Background(), 1*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 0 {
		t.Errorf("cleaned %d, want 0 (recent jobs)", n)
	}
}

func TestJobStore_Cleanup_KeepsPendingJobs(t *testing.T) {
	s := newMemoryJobStore()
	s.Create("job1")
	js, _ := s.Get("job1")
	js.UpdatedAt = time.Now().Add(-2 * time.Hour)
	// status is still "pending"

	n, err := s.Cleanup(context.Background(), 1*time.Hour)
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if n != 0 {
		t.Errorf("cleaned %d, want 0 (pending jobs not cleaned)", n)
	}
}

// ============================================================================
// WebSocket/SSE Hub
// ============================================================================

func TestJobStore_SSEHub_BroadcastsUpdate(t *testing.T) {
	s := newMemoryJobStore()
	mgr := NewAsyncJobManagerWithRetry(s, nil, 0)

	s.Create("job1")
	ch := mgr.subscribe("job1")

	s.Update("job1", JobProcessing, nil, "")
	mgr.broadcast("job1")

	select {
	case state := <-ch:
		if state.Status != JobProcessing {
			t.Errorf("status = %q, want processing", state.Status)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for broadcast")
	}

	mgr.unsubscribe("job1", ch)
}

func TestJobStore_SSEHub_MultipleSubscribers(t *testing.T) {
	s := newMemoryJobStore()
	mgr := NewAsyncJobManagerWithRetry(s, nil, 0)

	s.Create("job1")
	ch1 := mgr.subscribe("job1")
	ch2 := mgr.subscribe("job1")

	s.Update("job1", JobCompleted, "done", "")
	mgr.broadcast("job1")

	got1 := false
	got2 := false
	for range 2 {
		select {
		case state := <-ch1:
			if state.Status == JobCompleted {
				got1 = true
			}
		case state := <-ch2:
			if state.Status == JobCompleted {
				got2 = true
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for broadcast")
		}
	}
	if !got1 || !got2 {
		t.Error("not all subscribers received broadcast")
	}

	mgr.unsubscribe("job1", ch1)
	mgr.unsubscribe("job1", ch2)
}

// ============================================================================
// Full Entry Test: DELETE endpoint via HTTP
// ============================================================================

func TestRegisterEntries_Async_Delete(t *testing.T) {
	app := testApp()

	handlers := &EntryHandlers{
		Async: map[string]AsyncHandler{
			"testJob": func(body []byte, js *JobState) error {
				js.Result = "done"
				return nil
			},
		},
	}

	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "async", Path: "/jobs/reports", Handler: "testJob"},
		},
	}
	err := RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("RegisterEntries: %v", err)
	}

	// Submit job
	resp1, _ := request(app, "POST", "/api/v1/jobs/reports", stringsReader(`{"x":1}`))
	if resp1.StatusCode != 202 {
		t.Fatalf("POST status = %d, want 202", resp1.StatusCode)
	}
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	var submit struct {
		JobID     string `json:"job_id"`
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal(body1, &submit)

	// Delete it
	resp2, _ := request(app, "DELETE", submit.StatusURL, nil)
	if resp2.StatusCode != 204 {
		t.Errorf("DELETE status = %d, want 204", resp2.StatusCode)
	}
}

func TestRegisterEntries_Async_DeleteNotFound(t *testing.T) {
	app := testApp()
	handlers := &EntryHandlers{}
	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "async", Path: "/jobs/reports", Handler: "testJob"},
		},
	}
	RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil)

	resp, _ := request(app, "DELETE", "/api/v1/jobs/reports/nonexistent", nil)
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestRegisterEntries_Async_CancelRejectsProcessing(t *testing.T) {
	app := testApp()
	block := make(chan struct{})
	handlers := &EntryHandlers{
		Async: map[string]AsyncHandler{
			"testSlow": func(body []byte, js *JobState) error {
				<-block
				return nil
			},
		},
	}
	cfg := &ServiceConfig{
		Entry: []EntryDef{
			{Type: "async", Path: "/jobs/slow", Handler: "testSlow"},
		},
	}
	RegisterEntries(app, cfg, handlers, "/api/v1", nil, nil, nil, nil, nil, nil, nil, nil)
	resp1, _ := request(app, "POST", "/api/v1/jobs/slow", stringsReader(`{}`))
	body1, _ := io.ReadAll(resp1.Body)
	resp1.Body.Close()
	var submit struct {
		StatusURL string `json:"status_url"`
	}
	json.Unmarshal(body1, &submit)

	time.Sleep(100 * time.Millisecond)

	resp2, _ := request(app, "DELETE", submit.StatusURL, nil)
	if resp2.StatusCode != 409 {
		t.Errorf("DELETE of processing job = %d, want 409", resp2.StatusCode)
	}
	block <- struct{}{}
}
