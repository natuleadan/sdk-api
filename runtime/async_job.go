package runtime

import (
	"bufio"
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/runtime/errcode"
)

// AsyncHandler is a function that processes an async job.
type AsyncHandler func(body []byte, job *JobState) error

// AsyncJobManager coordinates async job creation, processing, and status retrieval.
type AsyncJobManager struct {
	store      JobStore
	processor  AsyncHandler
	maxRetry   int
	callback   *AsyncCallbackConf
	cleanupTTL time.Duration
	subs       map[string]map[chan JobState]struct{}
	subsMu     sync.RWMutex
}

func NewAsyncJobManager(store JobStore, processor AsyncHandler) *AsyncJobManager {
	return NewAsyncJobManagerWithRetry(store, processor, 0)
}

func NewAsyncJobManagerWithRetry(store JobStore, processor AsyncHandler, maxRetries int) *AsyncJobManager {
	return &AsyncJobManager{
		store: store, processor: processor, maxRetry: maxRetries,
		subs: make(map[string]map[chan JobState]struct{}),
	}
}

func generateJobID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		logx.Errorf("async: generate job id error: %v", err)
	}
	return "j_" + hex.EncodeToString(b)
}

// HandleSubmit returns a Fiber handler for POST requests that creates a job.
func (m *AsyncJobManager) HandleSubmit() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := generateJobID()
		js := m.store.Create(id)
		status := js.Status
		body := append([]byte{}, c.Body()...)
		if m.processor != nil {
			js.MaxRetries = m.maxRetry
			reqCtx := c.Context()
			go func() {
				m.store.Update(id, JobProcessing, nil, "")
				m.broadcast(id)
				dl := time.Now().Add(5 * time.Minute)
				js.ProcessingDeadline = &dl
				if err := m.processor(body, js); err != nil {
					m.store.Update(id, JobFailed, nil, err.Error())
				} else {
					m.store.Update(id, JobCompleted, js.Result, "")
				}
				m.broadcast(id)
				m.notifyCallback(reqCtx, id)
			}()
		}
		return c.Status(202).JSON(fiber.Map{
			"job_id":     id,
			"status":     status,
			"status_url": fmt.Sprintf("%s/%s", c.Path(), id),
		})
	}
}

// HandleStatus returns a Fiber handler for GET /path/:job_id requests.
func (m *AsyncJobManager) HandleStatus() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("job_id")
		js, ok := m.store.Get(id)
		if !ok {
			return errcode.ErrNotFound("job", id)
		}
		return c.JSON(js)
	}
}

// HandleCancel returns a Fiber handler for DELETE /path/:job_id requests.
func (m *AsyncJobManager) HandleCancel() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("job_id")
		js, ok := m.store.Get(id)
		if !ok {
			return errcode.ErrNotFound("job", id)
		}
		if js.Status == JobProcessing {
			return c.Status(409).JSON(fiber.Map{"error": "cannot cancel, job is processing"})
		}
		m.store.Delete(id)
		return c.SendStatus(204)
	}
}

// HandleStatusSSE returns a Fiber handler for SSE streaming of job status changes.
func (m *AsyncJobManager) HandleStatusSSE() fiber.Handler {
	return func(c fiber.Ctx) error {
		id := c.Params("job_id")

		js, ok := m.store.Get(id)
		if !ok {
			return errcode.ErrNotFound("job", id)
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")

		// Send current state
		data, _ := json.Marshal(js)
		if _, err := c.Write([]byte("data: " + string(data) + "\n\n")); err != nil {
			return nil
		}
		c.RequestCtx().SetBodyStreamWriter(func(w *bufio.Writer) {
			if js.Status == JobCompleted || js.Status == JobFailed {
				return
			}
			ch := m.subscribe(id)
			defer m.unsubscribe(id, ch)
			for state := range ch {
				data, _ := json.Marshal(state)
				if _, fErr := fmt.Fprintf(w, "data: %s\n\n", data); fErr != nil {
					return
				}
				if fErr := w.Flush(); fErr != nil {
					return
				}
				if state.Status == JobCompleted || state.Status == JobFailed {
					return
				}
			}
		})
		return nil
	}
}

func (m *AsyncJobManager) subscribe(id string) chan JobState {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	if m.subs[id] == nil {
		m.subs[id] = make(map[chan JobState]struct{})
	}
	ch := make(chan JobState, 8)
	m.subs[id][ch] = struct{}{}
	return ch
}

func (m *AsyncJobManager) unsubscribe(id string, ch chan JobState) {
	m.subsMu.Lock()
	defer m.subsMu.Unlock()
	if subs, ok := m.subs[id]; ok {
		delete(subs, ch)
		close(ch)
		if len(subs) == 0 {
			delete(m.subs, id)
		}
	}
}

func (m *AsyncJobManager) broadcast(id string) {
	m.subsMu.RLock()
	defer m.subsMu.RUnlock()
	if subs, ok := m.subs[id]; ok {
		js, ok := m.store.Get(id)
		if !ok {
			return
		}
		for ch := range subs {
			select {
			case ch <- *js:
			default:
			}
		}
	}
}

func (m *AsyncJobManager) notifyCallback(ctx context.Context, id string) {
	if m.callback == nil || m.callback.URL == "" {
		return
	}
	js, ok := m.store.Get(id)
	if !ok {
		return
	}
	payload, _ := json.Marshal(js)
	maxRetry := m.callback.Retry
	if maxRetry <= 0 {
		maxRetry = 3
	}
	retryDelay := parseDurationDef(m.callback.RetryDelay)
	if retryDelay <= 0 {
		retryDelay = 5 * time.Second
	}
	for attempt := 0; attempt <= maxRetry; attempt++ {
		if attempt > 0 {
			time.Sleep(retryDelay)
		}
		req, err := http.NewRequestWithContext(ctx, "POST", m.callback.URL, bytes.NewReader(payload))
		if err != nil {
			logx.Errorf("async callback: create request: %v", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if m.callback.Secret != "" {
			mac := hmac.New(sha256.New, []byte(m.callback.Secret))
			mac.Write(payload)
			sig := hex.EncodeToString(mac.Sum(nil))
			req.Header.Set("X-Job-Signature", sig)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			logx.Errorf("async callback attempt %d: %v", attempt+1, err)
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		if closeErr := resp.Body.Close(); closeErr != nil {
			logx.Errorf("async callback: close body: %v", closeErr)
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return
		}
		logx.Errorf("async callback attempt %d: status %d", attempt+1, resp.StatusCode)
	}
	logx.Errorf("async callback: all %d attempts failed for job %s", maxRetry+1, id)
}
