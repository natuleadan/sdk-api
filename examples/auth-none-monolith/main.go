package main

import (
	"context"
	"log"
	"sync"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

type Post struct {
	ID      int64  `db:"id,primary,auto" json:"id"`
	Title   string `db:"title,required"   json:"title"`
	Content string `db:"content"          json:"content"`
}

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	crud := &postCRUD{svc: svc}
	svc.WithCRUD("Post", crud)
	svc.RegisterModel("Post", (*Post)(nil))

	svc.WithRest("listAll", func(c fiber.Ctx) error {
		pgPool := crud.getPool()
		rows, err := pgPool.Query(c.Context(), "SELECT id, title, content FROM post ORDER BY id LIMIT 100")
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		defer rows.Close()
		items, err := pgx.CollectRows(rows, pgx.RowToStructByName[Post])
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
		return c.JSON(items)
	})

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

type postCRUD struct {
	svc   *runtime.Service
	once  sync.Once
	pool  *pgxpool.Pool
	table *db.Table[Post]
	inner runtime.CRUDProvider
}

func (p *postCRUD) getPool() *pgxpool.Pool {
	p.once.Do(func() {
		p.pool = p.svc.Pool("main").(*pgxpool.Pool)
	})
	return p.pool
}

func (p *postCRUD) getTable() *db.Table[Post] {
	p.getPool()
	if p.table == nil {
		var err error
		p.table, err = db.NewTable[Post](p.pool, "post")
		if err != nil {
			log.Fatalf("table: %v", err)
		}
		if err := p.table.AutoInit(context.Background()); err != nil {
			log.Fatalf("autoinit: %v", err)
		}
		p.inner = runtime.NewCRUDProvider(p.table, nil)
	}
	return p.table
}

func (p *postCRUD) lazy() runtime.CRUDProvider {
	p.getTable()
	return p.inner
}

func (p *postCRUD) List(c fiber.Ctx, params runtime.ListParams) error {
	return p.lazy().List(c, params)
}
func (p *postCRUD) Get(c fiber.Ctx, id string) error {
	return p.lazy().Get(c, id)
}
func (p *postCRUD) Create(c fiber.Ctx, body []byte) error {
	return p.lazy().Create(c, body)
}
func (p *postCRUD) Update(c fiber.Ctx, id string, body []byte) error {
	return p.lazy().Update(c, id, body)
}
func (p *postCRUD) Delete(c fiber.Ctx, id string) error {
	return p.lazy().Delete(c, id)
}

// fiber:context-methods migrated
