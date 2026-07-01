package runtime

import (
	"fmt"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/logx"
)

// NewCRUDProvider wraps a db.Table[T] (PostgreSQL) into a CRUDProvider.
func NewCRUDProvider[T any](table *db.Table[T], hooks EntryHooks[T]) CRUDProvider {
	if hooks == nil {
		var d DefaultHooks[T]
		hooks = d
	}
	return &tableCRUD[T]{table: table, hooks: hooks}
}

type tableCRUD[T any] struct {
	table *db.Table[T]
	hooks EntryHooks[T]
}

func (t *tableCRUD[T]) SetHooks(hooks any) {
	if h, ok := hooks.(EntryHooks[T]); ok {
		t.hooks = h
	}
}

func (t *tableCRUD[T]) List(ctx *fiber.Ctx, params ListParams) error {
	if len(params.Filters) > 0 {
		where := make(map[string]any, len(params.Filters))
		var wcs []db.ColumnValue
		for k, v := range params.Filters {
			where[k] = v
			wcs = append(wcs, db.Col(k, v))
		}
		items, err := t.table.QueryWhere(ctx.UserContext(), where, params.Sort, params.Size, (params.Page-1)*params.Size)
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		total, err := t.table.Count(ctx.UserContext(), wcs...)
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		return ctx.JSON(PaginatedResponse{Data: items, Total: total, Page: params.Page, Size: params.Size})
	}

	items, total, err := t.table.QueryPaginated(ctx.UserContext(), params.Page, params.Size, params.Sort)
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(PaginatedResponse{Data: items, Total: total, Page: params.Page, Size: params.Size})
}

func (t *tableCRUD[T]) Get(ctx *fiber.Ctx, id string) error {
	item, err := t.table.Get(ctx.UserContext(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(item)
}

func (t *tableCRUD[T]) Create(ctx *fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}

	var err error
	entity, err = t.hooks.BeforeCreate(ctx.UserContext(), entity)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}

	if err := t.table.Create(ctx.UserContext(), &entity); err != nil {
		return fiber.NewError(500, err.Error())
	}

	if err := t.hooks.AfterCreate(ctx.UserContext(), &entity); err != nil {
		logx.Errorf("after create hook: %v", err)
	}
	return ctx.Status(201).JSON(entity)
}

func (t *tableCRUD[T]) Update(ctx *fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}

	var err error
	patch, err = t.hooks.BeforeUpdate(ctx.UserContext(), id, patch)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}

	patch = t.table.ResolvePatch(patch)
	entity, err := t.table.Update(ctx.UserContext(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}

	if err := t.hooks.AfterUpdate(ctx.UserContext(), entity); err != nil {
		logx.Errorf("after update hook: %v", err)
	}
	return ctx.JSON(entity)
}

func (t *tableCRUD[T]) Delete(ctx *fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.UserContext(), id); err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Delete(ctx.UserContext(), id); err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterDelete(ctx.UserContext(), id); err != nil {
		logx.Errorf("after delete hook: %v", err)
	}
	return ctx.SendStatus(204)
}

// PaginatedResponse is used by tableCRUD.List.
type PaginatedResponse struct {
	Data  any   `json:"data"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// ---- MySQL CRUDProvider ----

func NewMySQLCRUDProvider[T any](table *db.MySQLTable[T], hooks EntryHooks[T]) CRUDProvider {
	if hooks == nil {
		var d DefaultHooks[T]
		hooks = d
	}
	return &mysqlCRUD[T]{table: table, hooks: hooks}
}

type mysqlCRUD[T any] struct {
	table *db.MySQLTable[T]
	hooks EntryHooks[T]
}

func (t *mysqlCRUD[T]) SetHooks(hooks any) {
	if h, ok := hooks.(EntryHooks[T]); ok {
		t.hooks = h
	}
}

func (t *mysqlCRUD[T]) List(ctx *fiber.Ctx, params ListParams) error {
	items, err := t.table.List(ctx.UserContext())
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	total := int64(len(items))
	start := max((params.Page-1)*params.Size, 0)
	if start > len(items) {
		start = len(items)
	}
	end := min(start+params.Size, len(items))
	return ctx.JSON(PaginatedResponse{Data: items[start:end], Total: total, Page: params.Page, Size: params.Size})
}

func (t *mysqlCRUD[T]) Get(ctx *fiber.Ctx, id string) error {
	item, err := t.table.Get(ctx.UserContext(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(item)
}

func (t *mysqlCRUD[T]) Create(ctx *fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	entity, err := t.hooks.BeforeCreate(ctx.UserContext(), entity)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Create(ctx.UserContext(), &entity); err != nil {
		return fiber.NewError(500, err.Error())
	}
	_ = t.hooks.AfterCreate(ctx.UserContext(), &entity)
	return ctx.Status(201).JSON(entity)
}

func (t *mysqlCRUD[T]) Update(ctx *fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	patch, err := t.hooks.BeforeUpdate(ctx.UserContext(), id, patch)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	entity, err := t.table.Update(ctx.UserContext(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	_ = t.hooks.AfterUpdate(ctx.UserContext(), entity)
	return ctx.JSON(entity)
}

func (t *mysqlCRUD[T]) Delete(ctx *fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.UserContext(), id); err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Delete(ctx.UserContext(), id); err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	_ = t.hooks.AfterDelete(ctx.UserContext(), id)
	return ctx.SendStatus(204)
}

// ---- Turso CRUDProvider ----

func NewTursoCRUDProvider[T any](table *db.TursoTable[T], hooks EntryHooks[T]) CRUDProvider {
	if hooks == nil {
		var d DefaultHooks[T]
		hooks = d
	}
	return &tursoCRUD[T]{table: table, hooks: hooks}
}

type tursoCRUD[T any] struct {
	table *db.TursoTable[T]
	hooks EntryHooks[T]
}

func (t *tursoCRUD[T]) SetHooks(hooks any) {
	if h, ok := hooks.(EntryHooks[T]); ok {
		t.hooks = h
	}
}

func (t *tursoCRUD[T]) List(ctx *fiber.Ctx, params ListParams) error {
	items, err := t.table.List(ctx.UserContext())
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	total := int64(len(items))
	start := max((params.Page-1)*params.Size, 0)
	if start > len(items) {
		start = len(items)
	}
	end := min(start+params.Size, len(items))
	return ctx.JSON(PaginatedResponse{Data: items[start:end], Total: total, Page: params.Page, Size: params.Size})
}

func (t *tursoCRUD[T]) Get(ctx *fiber.Ctx, id string) error {
	item, err := t.table.Get(ctx.UserContext(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(item)
}

func (t *tursoCRUD[T]) Create(ctx *fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	entity, err := t.hooks.BeforeCreate(ctx.UserContext(), entity)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Create(ctx.UserContext(), &entity); err != nil {
		return fiber.NewError(500, err.Error())
	}
	_ = t.hooks.AfterCreate(ctx.UserContext(), &entity)
	return ctx.Status(201).JSON(entity)
}

func (t *tursoCRUD[T]) Update(ctx *fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	patch, err := t.hooks.BeforeUpdate(ctx.UserContext(), id, patch)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	entity, err := t.table.Update(ctx.UserContext(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	_ = t.hooks.AfterUpdate(ctx.UserContext(), entity)
	return ctx.JSON(entity)
}

func (t *tursoCRUD[T]) Delete(ctx *fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.UserContext(), id); err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Delete(ctx.UserContext(), id); err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	_ = t.hooks.AfterDelete(ctx.UserContext(), id)
	return ctx.SendStatus(204)
}
