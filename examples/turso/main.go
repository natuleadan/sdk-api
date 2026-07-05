package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"turso-bench/models"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/proc"
	sm "github.com/natuleadan/sdk-api/server/middleware"
)

func main() {
	url := os.Getenv("TURSO_URL")
	if url == "" {
		url = "bench.db"
	}

	table, err := db.NewTursoTable[models.Product](url, "product")
	if err != nil {
		log.Fatalf("table: %v", err)
	}
	defer table.Close()

	if err := table.AutoInit(context.Background()); err != nil {
		log.Fatalf("autoinit: %v", err)
	}

	app := fiber.New(fiber.Config{})
	app.Use(sm.Logger())
	app.Use(sm.Recovery())

	app.Get("/api/v1/product/:id", func(c fiber.Ctx) error {
		id := c.Params("id")
		item, err := table.Get(c.Context(), id)
		if err != nil {
			if err == db.ErrNotFound {
				return c.Status(404).JSON(fiber.Map{"error": "not found"})
			}
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(item)
	})

	app.Post("/api/v1/products", func(c fiber.Ctx) error {
		var p models.Product
		if err := c.Bind().Body(&p); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if err := table.Create(c.Context(), &p); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(p)
	})

	port := 18086
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	proc.AddShutdownListener(func() {
		logx.Info("shutting down...")
		table.Close()
	})

	logx.Infof("turso-bench starting on :%d", port)
	log.Fatal(app.Listen(":" + strconv.Itoa(port)))
}

// fiber:context-methods migrated
