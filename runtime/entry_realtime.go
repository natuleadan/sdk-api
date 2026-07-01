package runtime

import (
	"bufio"
	"context"
	"fmt"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"

	sm "github.com/natuleadan/sdk-api/server/middleware"
)

func registerWebSocket(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string) error {
	h, ok := handlers.WS[entry.Handler]
	if !ok {
		return fmt.Errorf("websocket handler %q not found", entry.Handler)
	}
	path := prefix + entry.Path
	handler := h
	app.Get(path, sm.WebSocket(func(c *websocket.Conn) {
		_ = handler(context.Background(), c)
	}))
	return nil
}

func registerSSE(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string) error {
	h, ok := handlers.SSE[entry.Handler]
	if !ok {
		return fmt.Errorf("sse handler %q not found", entry.Handler)
	}
	path := prefix + entry.Path
	handler := h
	app.Get(path, func(c *fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		ctx := c.UserContext()
		c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
			send := func(data string) {
				_, _ = w.WriteString(data)
				_, _ = w.WriteString("\n")
				_ = w.Flush()
			}
			_ = handler(ctx, send)
		})
		return nil
	})
	return nil
}
