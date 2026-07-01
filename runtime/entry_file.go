package runtime

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
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
		if len(entry.AllowedTypes) > 0 && !isContentTypeAllowed(contentType, entry.AllowedTypes) {
			return fiber.NewError(415, fmt.Sprintf("content-type %q not allowed", contentType))
		}

		if entry.MaxSize != "" {
			maxBytes := parseMaxSize(entry.MaxSize)
			if maxBytes > 0 && len(c.Body()) > maxBytes {
				return fiber.NewError(413, "request body too large")
			}
		}

		if entry.MaxFiles > 0 {
			form, formErr := c.MultipartForm()
			if formErr == nil && len(form.File) > entry.MaxFiles {
				return fiber.NewError(413, "too many files")
			}
		}

		if entry.MagicBytes && len(c.Body()) > 512 {
			detected := http.DetectContentType(c.Body())
			if !isContentTypeAllowed(detected, entry.AllowedTypes) {
				return fiber.NewError(415, fmt.Sprintf("file content type %q does not match allowed types", detected))
			}
		}

		return c.Next()
	}
}

func isContentTypeAllowed(contentType string, allowed []string) bool {
	for _, t := range allowed {
		if matchContentType(contentType, t) {
			return true
		}
	}
	return false
}

func matchContentType(contentType, allowed string) bool {
	// exact match or wildcard match
	if contentType == allowed {
		return true
	}
	if before, ok := strings.CutSuffix(allowed, "/*"); ok {
		prefix := before
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
	_, _ = fmt.Sscanf(s, "%d", &n)
	return n * multiplier
}

// SanitizeFilename removes dangerous characters from filenames.
// - Removes path separators (/, \)
// - Removes null bytes
// - Limits length to 255 chars
// - Only allows [a-zA-Z0-9._-]
func SanitizeFilename(name string) string {
	if name == "" {
		return "untitled"
	}
	ext := filepath.Ext(name)
	baseName := name
	if ext != "" {
		baseName = name[:len(name)-len(ext)]
	}
	baseName = strings.ReplaceAll(baseName, "/", "")
	baseName = strings.ReplaceAll(baseName, "\\", "")
	baseName = strings.ReplaceAll(baseName, "\x00", "")
	var safe strings.Builder
	for _, r := range baseName {
		if isSafeFileChar(r) {
			safe.WriteRune(r)
		}
	}
	result := safe.String()
	if result == "" {
		result = "untitled"
	}
	safeExt := sanitizeExt(ext)
	result += safeExt
	if len(result) > 255 {
		result = result[:255]
	}
	if result == "" {
		return "untitled"
	}
	return result
}

func isSafeFileChar(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
}

func sanitizeExt(ext string) string {
	var safe strings.Builder
	for _, r := range ext {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			safe.WriteRune(r)
		}
	}
	return safe.String()
}
