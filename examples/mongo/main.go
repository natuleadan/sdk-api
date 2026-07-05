package main

import (
	"log"
	"os"
	"strconv"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/proc"
	"github.com/natuleadan/sdk-api/infra/stores/mon"
	sm "github.com/natuleadan/sdk-api/server/middleware"
	"go.mongodb.org/mongo-driver/v2/bson"
)

type Product struct {
	ID    int     `bson:"_id" json:"id"`
	Name  string  `bson:"name" json:"name"`
	Price float64 `bson:"price" json:"price"`
	Stock int     `bson:"stock" json:"stock"`
}

func main() {
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017"
	}

	model := mon.MustNewModel(uri, "bench", "products")

	app := fiber.New(fiber.Config{})
	app.Use(sm.Logger())
	app.Use(sm.Recovery())

	app.Get("/api/v1/product/:id", func(c fiber.Ctx) error {
		id, err := strconv.Atoi(c.Params("id"))
		if err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid id"})
		}
		var p Product
		if err := model.FindOne(c.Context(), &p, bson.M{"_id": id}); err != nil {
			return c.Status(404).JSON(fiber.Map{"error": "not found"})
		}
		return c.JSON(p)
	})

	app.Post("/api/v1/products", func(c fiber.Ctx) error {
		var p Product
		if err := c.Bind().Body(&p); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": err.Error()})
		}
		if _, err := model.InsertOne(c.Context(), &p); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.Status(201).JSON(p)
	})

	port := 18087
	if p := os.Getenv("PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			port = v
		}
	}

	proc.AddShutdownListener(func() {
		logx.Info("shutting down...")
	})

	logx.Infof("mongo-bench starting on :%d", port)
	log.Fatal(app.Listen(":" + strconv.Itoa(port)))
}

// fiber:context-methods migrated
