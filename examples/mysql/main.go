package main

import (
	"log"
	"os"

	"mysql-bench/models"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

func main() {
	cfgPath := os.Getenv("CONFIG_PATH")
	if cfgPath == "" {
		cfgPath = "service.yaml"
	}
	svc, err := runtime.New(cfgPath)
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	pgPool := svc.Pool("pg-main").(*pgxpool.Pool)
	table, err := db.NewTable[models.Product](pgPool, "product")
	if err != nil {
		log.Fatalf("table: %v", err)
	}

	svc.WithCRUD("Product", runtime.NewCRUDProvider(table, nil))

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}
