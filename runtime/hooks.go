package runtime

import (
	"context"
	"fmt"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

// EntryHooks defines lifecycle callbacks for entry endpoints (HTTP).
type EntryHooks[T any] interface {
	BeforeCreate(ctx context.Context, req T) (T, error)
	AfterCreate(ctx context.Context, entity *T) error
	BeforeUpdate(ctx context.Context, id any, patch map[string]any) (map[string]any, error)
	AfterUpdate(ctx context.Context, entity *T) error
	BeforeDelete(ctx context.Context, id any) error
	AfterDelete(ctx context.Context, id any) error
	BeforeTransform(ctx context.Context, req T) (T, error)
	AfterTransform(ctx context.Context, result any) error
}

type DefaultHooks[T any] struct{}

func (DefaultHooks[T]) BeforeCreate(_ context.Context, req T) (T, error)           { return req, nil }
func (DefaultHooks[T]) AfterCreate(_ context.Context, _ *T) error                  { return nil }
func (DefaultHooks[T]) BeforeUpdate(_ context.Context, _ any, patch map[string]any) (map[string]any, error) {
	return patch, nil
}
func (DefaultHooks[T]) AfterUpdate(_ context.Context, _ *T) error  { return nil }
func (DefaultHooks[T]) BeforeDelete(_ context.Context, _ any) error { return nil }
func (DefaultHooks[T]) AfterDelete(_ context.Context, _ any) error  { return nil }
func (DefaultHooks[T]) BeforeTransform(_ context.Context, req T) (T, error) { return req, nil }
func (DefaultHooks[T]) AfterTransform(_ context.Context, _ any) error       { return nil }

// ExitHooks defines lifecycle callbacks for exit workers (NATS).
type ExitHooks interface {
	OnMessage(ctx context.Context, msg []byte) ([]byte, error)
	OnSuccess(ctx context.Context)
	OnError(ctx context.Context, err error)
}

type DefaultExitHooks struct{}

func (DefaultExitHooks) OnMessage(_ context.Context, msg []byte) ([]byte, error) { return msg, nil }
func (DefaultExitHooks) OnSuccess(_ context.Context)                              {}
func (DefaultExitHooks) OnError(_ context.Context, _ error)                       {}

// SSEHandler is called when an SSE client connects.
type SSEHandler func(ctx context.Context, send func(data string)) error

// WSHandler is called when a WebSocket client connects.
type WSHandler func(ctx context.Context, conn *websocket.Conn) error

// WrapTransformHandler wraps a REST handler with BeforeTransform/AfterTransform hooks.
// The hooks must implement EntryHooks[T] where T is the request model.
// BeforeTransform parses the request body into T, calls the hook, stores the result
// in c.Locals("transformed"), then executes the handler. AfterTransform is called
// with the response body after the handler completes.
//
// Usage:
//
//	svc.WithRest("onTransform", runtime.WrapTransformHandler(
//	    func(c *fiber.Ctx) error {
//	        input := c.Locals("transformed").(Product)
//	        return c.JSON(fiber.Map{"name": input.Name})
//	    },
//	    &ProductHooks{},
//	))
func WrapTransformHandler[T any](handler func(*fiber.Ctx) error, hooks EntryHooks[T]) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		var input T
		method := c.Method()
		if method != "GET" && method != "DELETE" && len(c.Body()) > 0 {
			if err := c.BodyParser(&input); err != nil {
				return fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("invalid body: %v", err))
			}
		}
		transformed, err := hooks.BeforeTransform(c.UserContext(), input)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, err.Error())
		}
		c.Locals("transformed", transformed)

		if err := handler(c); err != nil {
			return err
		}

		return hooks.AfterTransform(c.UserContext(), string(c.Response().Body()))
	}
}
