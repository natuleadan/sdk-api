//go:build !race

package middleware

import (
	"net/http"
	"testing"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func TestWebSocketHandler(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Get("/ws", WebSocket(func(c *websocket.Conn) {
		c.WriteMessage(websocket.TextMessage, []byte("hello"))
	}))

	req, _ := http.NewRequest("GET", "/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	resp, _ := app.Test(req)
	if resp.StatusCode == 404 {
		t.Error("expected WebSocket route to be registered, got 404")
	}
}

func TestWebSocketWithConfig(t *testing.T) {
	logx.Disable()
	app := fiber.New()
	app.Get("/ws", WebSocketWithConfig(WebSocketConfig{
		Origins: []string{"https://example.com"},
	}, func(c *websocket.Conn) {
		c.WriteMessage(websocket.TextMessage, []byte("ok"))
	}))

	req, _ := http.NewRequest("GET", "/ws", nil)
	resp, _ := app.Test(req)
	if resp.StatusCode == 404 {
		t.Error("expected WebSocket route to be registered, got 404")
	}
}

func TestWebSocketHandlerNil(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered from nil handler: %v", r)
		}
	}()
	_ = WebSocket(nil)
}
