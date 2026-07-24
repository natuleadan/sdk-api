package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/events"
)

func registerAsync(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, mws []fiber.Handler, store JobStore, brokers map[string]events.EventBroker) error {
	path := resolveAsyncPath(prefix, entry)
	if path == "" {
		return fmt.Errorf("async entry requires path or resource")
	}

	processor := resolveAsyncHandler(handlers.Async, entry.Handler)
	if entry.Handler != "" && processor == nil {
		return fmt.Errorf("async handler %q not found", entry.Handler)
	}

	maxRetries, ttl, _ := setupAsyncReaper(entry, store)

	mgr := NewAsyncJobManagerWithRetry(store, processor, maxRetries)
	if entry.AsyncStore != nil {
		mgr.callback = entry.AsyncStore.Callback
		mgr.cleanupTTL = ttl
		mgr.processingTimeout = parseDurationDef(entry.AsyncStore.Reassign.ProcessingTimeout)
		if mgr.processingTimeout <= 0 {
			mgr.processingTimeout = 5 * time.Minute
		}
		if entry.AsyncStore.MaxConcurrent > 0 {
			mgr.sem = make(chan struct{}, entry.AsyncStore.MaxConcurrent)
		}
	}

	submitHandler := mgr.HandleSubmit()
	pubTargets := getPublishTargets(entry)
	if len(pubTargets) > 0 && len(brokers) > 0 {
		submitHandler = wrapEventPublish(context.Background(), submitHandler, pubTargets, entry.EventStream, brokers)
	}

	registerWithMws(app, "POST", path, mws, submitHandler)
	registerWithMws(app, "GET", path+"/:job_id", mws, mgr.HandleStatus())
	registerWithMws(app, "DELETE", path+"/:job_id", mws, mgr.HandleCancel())
	registerWithMws(app, "GET", path+"/:job_id/status", mws, mgr.HandleStatusSSE())
	registerWithMws(app, "GET", path, mws, mgr.HandleList())

	return nil
}

func resolveAsyncPath(prefix string, entry *EntryDef) string {
	rsc := entry.Resource
	if entry.Path != "" {
		rsc = entry.Path
	}
	if rsc == "" {
		return ""
	}
	return prefix + rsc
}

func setupAsyncReaper(entry *EntryDef, store JobStore) (int, time.Duration, *Reaper) {
	if entry.AsyncStore == nil {
		return 0, 0, nil
	}
	ttl := parseDurationDef(entry.AsyncStore.ResultTTL)
	if entry.AsyncStore.Reassign == nil || !entry.AsyncStore.Reassign.Enabled {
		return 0, ttl, nil
	}
	maxRetries := entry.AsyncStore.Reassign.MaxRetries
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
	reaper := NewReaper(store, timeout, interval, maxRetries)
	reaper.cleanupTTL = ttl
	reaper.Start()
	return maxRetries, ttl, reaper
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
