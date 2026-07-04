package main

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/runtime"
)

type User struct {
	ID    int64  `db:"id,primary,auto" json:"id"`
	Name  string `db:"name,required"   json:"name"`
	Email string `db:"email,unique"    json:"email"`
}

func main() {
	svc, err := runtime.New("service.yaml")
	if err != nil {
		log.Fatalf("init: %v", err)
	}

	svc.WithCRUD("User", newUserCRUD(svc))
	svc.RegisterModel("User", (*User)(nil))

	if err := svc.Run(); err != nil {
		log.Fatalf("run: %v", err)
	}
}

type userCRUD struct {
	init  func() runtime.CRUDProvider
	inner runtime.CRUDProvider
}

func newUserCRUD(svc *runtime.Service) *userCRUD {
	return &userCRUD{
		init: func() runtime.CRUDProvider {
			pgPool := svc.Pool("main").(*pgxpool.Pool)
			table, err := db.NewTable[User](pgPool, "usersvc")
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

func (u *userCRUD) get() runtime.CRUDProvider {
	if u.inner == nil {
		u.inner = u.init()
	}
	return u.inner
}

func (u *userCRUD) List(c *fiber.Ctx, params runtime.ListParams) error {
	return u.get().List(c, params)
}
func (u *userCRUD) Get(c *fiber.Ctx, id string) error {
	return u.get().Get(c, id)
}
func (u *userCRUD) Create(c *fiber.Ctx, body []byte) error {
	return u.get().Create(c, body)
}
func (u *userCRUD) Update(c *fiber.Ctx, id string, body []byte) error {
	return u.get().Update(c, id, body)
}
func (u *userCRUD) Delete(c *fiber.Ctx, id string) error {
	return u.get().Delete(c, id)
}
