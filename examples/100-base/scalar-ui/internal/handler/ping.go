package handler

import (
	"context"
	"fmt"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/runtime"
)

// Rest handlers ---------------------------------------------------------------

func Ping() func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.SendString("pong")
	}
}

func Echo() func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.SendString("echo")
	}
}

func Upload() func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.SendString("uploaded")
	}
}

func Status() func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.SendString("healthy")
	}
}

func Sub() func(*runtime.RestCtx) error {
	return func(c *runtime.RestCtx) error {
		return c.SendString("subdomain ok")
	}
}

// Realtime handlers -----------------------------------------------------------

func Chat() func(ctx context.Context, conn *websocket.Conn) error {
	return func(ctx context.Context, conn *websocket.Conn) error {
		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				return err
			}
			if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return err
			}
		}
	}
}

func Events() func(ctx context.Context, send func(data string)) error {
	return func(ctx context.Context, send func(data string)) error {
		for i := 0; i < 3; i++ {
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(100 * time.Millisecond):
				send(fmt.Sprintf("event-%d", i))
			}
		}
		return nil
	}
}

// Async handler ---------------------------------------------------------------

func Job(body []byte, job *runtime.JobState) error {
	job.Status = "done"
	return nil
}

// CRUD provider (in-memory) ---------------------------------------------------

type Product struct {
	ID    string `json:"id" db:"id"`
	Name  string `json:"name" db:"name"`
	Price int    `json:"price" db:"price"`
}

// ProductCRUD is an in-memory CRUD provider for the /products endpoint.
type ProductCRUD struct {
	items map[string]Product
	seq   int
}

func NewProductCRUD() runtime.CRUDProvider {
	return &ProductCRUD{items: map[string]Product{}}
}

func (p *ProductCRUD) List(c fiber.Ctx, params runtime.ListParams) error {
	out := make([]Product, 0, len(p.items))
	for _, v := range p.items {
		out = append(out, v)
	}
	return c.JSON(fiber.Map{"items": out})
}

func (p *ProductCRUD) Get(c fiber.Ctx, id string) error {
	item, ok := p.items[id]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	return c.JSON(item)
}

func (p *ProductCRUD) Create(c fiber.Ctx, body []byte) error {
	p.seq++
	id := fmt.Sprintf("prod-%d", p.seq)
	p.items[id] = Product{ID: id, Name: fmt.Sprintf("product-%d", p.seq), Price: p.seq * 10}
	return c.JSON(fiber.Map{"id": id})
}

func (p *ProductCRUD) Update(c fiber.Ctx, id string, body []byte) error {
	item, ok := p.items[id]
	if !ok {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{"error": "not found"})
	}
	item.Price++
	p.items[id] = item
	return c.JSON(item)
}

func (p *ProductCRUD) Delete(c fiber.Ctx, id string) error {
	delete(p.items, id)
	return c.SendStatus(fiber.StatusNoContent)
}
