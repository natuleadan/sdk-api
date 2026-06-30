package runtime

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/events"
)

func registerFile(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker) error {
	h := resolveHandler(handlers.Rest, entry.Handler)
	if h == nil {
		return fmt.Errorf("file handler %q not found", entry.Handler)
	}
	path := prefix + entry.Path

	// Apply file validation middleware if allowed_types or max_size specified
	if len(entry.AllowedTypes) > 0 || entry.MaxSize != "" {
		validate := fileValidator(entry)
		switch entry.Method {
		case "POST", "PUT", "PATCH":
			app.Use(path, validate)
		}
	}

	// Wrap with event_publish if configured
	targets := getPublishTargets(entry)
	if len(targets) > 0 && len(brokers) > 0 {
		h = wrapEventPublish(context.Background(), h, targets, entry.EventStream, brokers)
	}

	switch entry.Method {
	case "GET":
		app.Get(path, h)
	case "POST":
		app.Post(path, h)
	case "PUT":
		app.Put(path, h)
	case "PATCH":
		app.Patch(path, h)
	case "DELETE":
		app.Delete(path, h)
	default:
		return fmt.Errorf("unsupported HTTP method %q for file endpoint", entry.Method)
	}
	return nil
}

func fileValidator(entry *EntryDef) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		contentType := string(c.Request().Header.ContentType())
		if len(entry.AllowedTypes) > 0 {
			allowed := false
			for _, t := range entry.AllowedTypes {
				if matchContentType(contentType, t) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fiber.NewError(415, fmt.Sprintf("content-type %q not allowed", contentType))
			}
		}

		if entry.MaxSize != "" {
			maxBytes := parseMaxSize(entry.MaxSize)
			if maxBytes > 0 && len(c.Body()) > maxBytes {
				return fiber.NewError(413, "request body too large")
			}
		}

		// Magic byte verification: check actual content matches declared type
		if entry.MagicBytes && len(c.Body()) > 512 {
			detected := http.DetectContentType(c.Body())
			allowed := false
			for _, t := range entry.AllowedTypes {
				if matchContentType(detected, t) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fiber.NewError(415, fmt.Sprintf("file content type %q does not match allowed types", detected))
			}
		}

		return c.Next()
	}
}

func matchContentType(contentType, allowed string) bool {
	// exact match or wildcard match
	if contentType == allowed {
		return true
	}
	if strings.HasSuffix(allowed, "/*") {
		prefix := strings.TrimSuffix(allowed, "/*")
		return strings.HasPrefix(contentType, prefix+"/")
	}
	return false
}

func parseMaxSize(s string) int {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return 0
	}
	multiplier := 1
	switch {
	case strings.HasSuffix(s, "mb"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "mb")
	case strings.HasSuffix(s, "kb"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "kb")
	case strings.HasSuffix(s, "gb"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gb")
	case strings.HasSuffix(s, "b"):
		multiplier = 1
		s = strings.TrimSuffix(s, "b")
	}
	var n int
	fmt.Sscanf(s, "%d", &n)
	return n * multiplier
}
