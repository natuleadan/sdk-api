package middleware

import (
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

type WebSocketConfig struct {
	Origins          []string
	HandshakeTimeout time.Duration
	ReadBufferSize   int
	WriteBufferSize  int
}

func WebSocket(handler func(*websocket.Conn)) fiber.Handler {
	return WebSocketWithConfig(WebSocketConfig{}, handler)
}

func WebSocketWithConfig(cfg WebSocketConfig, handler func(*websocket.Conn)) fiber.Handler {
	wsCfg := websocket.Config{
		Filter: func(c *fiber.Ctx) bool {
			return true
		},
	}
	if len(cfg.Origins) > 0 {
		wsCfg.Origins = cfg.Origins
	}
	if cfg.HandshakeTimeout > 0 {
		wsCfg.HandshakeTimeout = cfg.HandshakeTimeout
	}
	if cfg.ReadBufferSize > 0 {
		wsCfg.ReadBufferSize = cfg.ReadBufferSize
	}
	if cfg.WriteBufferSize > 0 {
		wsCfg.WriteBufferSize = cfg.WriteBufferSize
	}
	return websocket.New(handler, wsCfg)
}
