package main

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

type Product struct {
	ID    int64   `db:"id,primary,auto" json:"id"`
	Name  string  `db:"name,required"   json:"name"`
	Price float64 `db:"price"           json:"price"`
}

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svc.WithCRUD("Product", newProductCRUD(svc))
	svc.RegisterModel("Product", (*Product)(nil))

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

type productCRUD struct {
	init  func() runtime.CRUDProvider
	inner runtime.CRUDProvider
}

func newProductCRUD(svc *runtime.Service) *productCRUD {
	return &productCRUD{
		init: func() runtime.CRUDProvider {
			pgPool := svc.Pool("main").(*pgxpool.Pool)
			table, err := db.NewTable[Product](pgPool, "product")
			if err != nil {
				log.Fatalf("table: %v", err)
			}
			if err := table.AutoInit(context.Background()); err != nil {
				log.Fatalf("autoinit: %v", err)
			}
			return runtime.NewCRUDProvider(table, nil)
		},
	}
}

func (p *productCRUD) get() runtime.CRUDProvider {
	if p.inner == nil {
		p.inner = p.init()
	}
	return p.inner
}

func (p *productCRUD) List(c *fiber.Ctx, params runtime.ListParams) error {
	return p.get().List(c, params)
}
func (p *productCRUD) Get(c *fiber.Ctx, id string) error {
	return p.get().Get(c, id)
}
func (p *productCRUD) Create(c *fiber.Ctx, body []byte) error {
	return p.get().Create(c, body)
}
func (p *productCRUD) Update(c *fiber.Ctx, id string, body []byte) error {
	return p.get().Update(c, id, body)
}
func (p *productCRUD) Delete(c *fiber.Ctx, id string) error {
	return p.get().Delete(c, id)
}
