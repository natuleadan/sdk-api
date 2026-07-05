package runtime

import (
	"fmt"

	"github.com/gofiber/fiber/v3"
)

func registerAsync(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string) error {
	path := prefix + entry.Path
	store := newMemoryJobStore()

	var processor AsyncHandler
	if entry.Handler != "" {
		h := resolveAsyncHandler(handlers.Async, entry.Handler)
		if h == nil {
			return fmt.Errorf("async handler %q not found", entry.Handler)
		}
		processor = h
	}

	mgr := NewAsyncJobManager(store, processor)

	app.Post(path, mgr.HandleSubmit())
	app.Get(path+"/:job_id", mgr.HandleStatus())

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
