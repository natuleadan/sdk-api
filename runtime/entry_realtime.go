package runtime

import (
	"bufio"
	"context"
	"fmt"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"

	"github.com/natuleadan/sdk-api/infra/logx"
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
		if err := handler(context.Background(), c); err != nil {
			logx.Errorf("websocket handler error: %v", err)
		}
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
	app.Get(path, func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		ctx := c.Context()
		c.RequestCtx().SetBodyStreamWriter(func(w *bufio.Writer) {
			send := func(data string) {
				if _, err := w.WriteString(data); err != nil {
					logx.Errorf("sse write string error: %v", err)
				}
				if _, err := w.WriteString("\n"); err != nil {
					logx.Errorf("sse write newline error: %v", err)
				}
				if err := w.Flush(); err != nil {
					logx.Errorf("sse flush error: %v", err)
				}
			}
			if err := handler(ctx, send); err != nil {
				logx.Errorf("sse handler error: %v", err)
			}
		})
		return nil
	})
	return nil
}

// fiber:context-methods migrated
