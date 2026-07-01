package runtime

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/gofiber/fiber/v2"
)

// AsyncHandler is a function that processes an async job.
// body contains the raw request body from the POST.
// job holds the job state — mutate job.Result before returning.
type AsyncHandler func(body []byte, job *JobState) error

// AsyncJobManager coordinates async job creation, processing, and status retrieval.
type AsyncJobManager struct {
	store     JobStore
	processor AsyncHandler
}

// NewAsyncJobManager creates a new manager with the given store and processor.
func NewAsyncJobManager(store JobStore, processor AsyncHandler) *AsyncJobManager {
	return &AsyncJobManager{
		store:     store,
		processor: processor,
	}
}

func generateJobID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "j_" + hex.EncodeToString(b)
}

// HandleSubmit returns a Fiber handler for POST requests that creates a job.
func (m *AsyncJobManager) HandleSubmit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := generateJobID()
	js := m.store.Create(id)
	status := js.Status
	body := append([]byte{}, c.Body()...) // copy: safe for goroutine

	if m.processor != nil {
		go func() {
			m.store.Update(id, JobProcessing, nil, "")
			if err := m.processor(body, js); err != nil {
				m.store.Update(id, JobFailed, nil, err.Error())
			} else {
				m.store.Update(id, JobCompleted, js.Result, "")
			}
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
	return func(c *fiber.Ctx) error {
		id := c.Params("job_id")
		js, ok := m.store.Get(id)
		if !ok {
			return fiber.NewError(404, "job not found")
		}
		return c.JSON(js)
	}
}
