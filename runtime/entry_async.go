package runtime

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
)

func registerAsync(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, mws []fiber.Handler, store JobStore) error {
	path := prefix + entry.Path

	var processor AsyncHandler
	if entry.Handler != "" {
		h := resolveAsyncHandler(handlers.Async, entry.Handler)
		if h == nil {
			return fmt.Errorf("async handler %q not found", entry.Handler)
		}
		processor = h
	}

	maxRetries := 0
	var ttl time.Duration
	var reaper *Reaper
	if entry.AsyncStore != nil {
		if entry.AsyncStore.Reassign != nil && entry.AsyncStore.Reassign.Enabled {
			maxRetries = entry.AsyncStore.Reassign.MaxRetries
			if maxRetries <= 0 {
				maxRetries = 3
			}
			timeout := parseDurationDef(entry.AsyncStore.Reassign.ProcessingTimeout)
			if timeout <= 0 {
				timeout = 5 * time.Minute
			}
			interval := parseDurationDef(entry.AsyncStore.Reassign.ReapInterval)
			if interval <= 0 {
				interval = 30 * time.Second
			}
			reaper = NewReaper(store, timeout, interval, maxRetries)
			reaper.cleanupTTL = ttl
			reaper.Start()
		}
		ttl = parseDurationDef(entry.AsyncStore.ResultTTL)
	}

	mgr := NewAsyncJobManagerWithRetry(store, processor, maxRetries)
	if entry.AsyncStore != nil {
		mgr.callback = entry.AsyncStore.Callback
		mgr.cleanupTTL = ttl
	}

	registerWithMws(app, "POST", path, mws, mgr.HandleSubmit())
	registerWithMws(app, "GET", path+"/:job_id", mws, mgr.HandleStatus())
	registerWithMws(app, "DELETE", path+"/:job_id", mws, mgr.HandleCancel())
	registerWithMws(app, "GET", path+"/:job_id/status", mws, mgr.HandleStatusSSE())

	return nil
}

func resolveAsyncHandler(handlers map[string]AsyncHandler, name string) AsyncHandler {
	if handlers == nil {
		return nil
	}
	h, ok := handlers[name]
	if !ok {
		return nil
	}
	return h
}
