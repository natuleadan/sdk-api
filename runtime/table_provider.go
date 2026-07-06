package runtime

import (
	"fmt"

	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/stores/mon"
	"go.mongodb.org/mongo-driver/v2/bson"
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

func (t *tableCRUD[T]) List(ctx fiber.Ctx, params ListParams) error {
	if len(params.Filters) > 0 {
		where := make(map[string]any, len(params.Filters))
		var wcs []db.ColumnValue
		for k, v := range params.Filters {
			where[k] = v
			wcs = append(wcs, db.Col(k, v))
		}
		items, err := t.table.QueryWhere(ctx.Context(), where, params.Sort, params.Size, (params.Page-1)*params.Size)
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		total, err := t.table.Count(ctx.Context(), wcs...)
		if err != nil {
			return fiber.NewError(500, err.Error())
		}
		return ctx.JSON(PaginatedResponse{Data: items, Total: total, Page: params.Page, Size: params.Size})
	}

	items, total, err := t.table.QueryPaginated(ctx.Context(), params.Page, params.Size, params.Sort)
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(PaginatedResponse{Data: items, Total: total, Page: params.Page, Size: params.Size})
}

func (t *tableCRUD[T]) Get(ctx fiber.Ctx, id string) error {
	item, err := t.table.Get(ctx.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(item)
}

func (t *tableCRUD[T]) Create(ctx fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}

	var err error
	entity, err = t.hooks.BeforeCreate(ctx.Context(), entity)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}

	if err := t.table.Create(ctx.Context(), &entity); err != nil {
		return fiber.NewError(500, err.Error())
	}

	if err := t.hooks.AfterCreate(ctx.Context(), &entity); err != nil {
		logx.Errorf("after create hook: %v", err)
	}
	return ctx.Status(201).JSON(entity)
}

func (t *tableCRUD[T]) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}

	var err error
	patch, err = t.hooks.BeforeUpdate(ctx.Context(), id, patch)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}

	patch = t.table.ResolvePatch(patch)
	entity, err := t.table.Update(ctx.Context(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}

	if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
		logx.Errorf("after update hook: %v", err)
	}
	return ctx.JSON(entity)
}

func (t *tableCRUD[T]) Delete(ctx fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.Context(), id); err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Delete(ctx.Context(), id); err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterDelete(ctx.Context(), id); err != nil {
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

func (t *mysqlCRUD[T]) List(ctx fiber.Ctx, params ListParams) error {
	items, err := t.table.List(ctx.Context())
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	total := int64(len(items))
	start := min(max((params.Page-1)*params.Size, 0), len(items))
	end := min(start+params.Size, len(items))
	return ctx.JSON(PaginatedResponse{Data: items[start:end], Total: total, Page: params.Page, Size: params.Size})
}

func (t *mysqlCRUD[T]) Get(ctx fiber.Ctx, id string) error {
	item, err := t.table.Get(ctx.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(item)
}

func (t *mysqlCRUD[T]) Create(ctx fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	entity, err := t.hooks.BeforeCreate(ctx.Context(), entity)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Create(ctx.Context(), &entity); err != nil {
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterCreate(ctx.Context(), &entity); err != nil {
		logx.Errorf("crud: after create hook error: %v", err)
	}
	return ctx.Status(201).JSON(entity)
}

func (t *mysqlCRUD[T]) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	patch, err := t.hooks.BeforeUpdate(ctx.Context(), id, patch)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	entity, err := t.table.Update(ctx.Context(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
		logx.Errorf("crud: after update hook error: %v", err)
	}
	return ctx.JSON(entity)
}

func (t *mysqlCRUD[T]) Delete(ctx fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.Context(), id); err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Delete(ctx.Context(), id); err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterDelete(ctx.Context(), id); err != nil {
		logx.Errorf("crud: after delete hook error: %v", err)
	}
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

func (t *tursoCRUD[T]) List(ctx fiber.Ctx, params ListParams) error {
	items, err := t.table.List(ctx.Context())
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	total := int64(len(items))
	start := min(max((params.Page-1)*params.Size, 0), len(items))
	end := min(start+params.Size, len(items))
	return ctx.JSON(PaginatedResponse{Data: items[start:end], Total: total, Page: params.Page, Size: params.Size})
}

func (t *tursoCRUD[T]) Get(ctx fiber.Ctx, id string) error {
	item, err := t.table.Get(ctx.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(item)
}

func (t *tursoCRUD[T]) Create(ctx fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	entity, err := t.hooks.BeforeCreate(ctx.Context(), entity)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Create(ctx.Context(), &entity); err != nil {
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterCreate(ctx.Context(), &entity); err != nil {
		logx.Errorf("crud: after create hook error: %v", err)
	}
	return ctx.Status(201).JSON(entity)
}

func (t *tursoCRUD[T]) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	patch, err := t.hooks.BeforeUpdate(ctx.Context(), id, patch)
	if err != nil {
		return fiber.NewError(400, err.Error())
	}
	entity, err := t.table.Update(ctx.Context(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
		logx.Errorf("crud: after update hook error: %v", err)
	}
	return ctx.JSON(entity)
}

func (t *tursoCRUD[T]) Delete(ctx fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.Context(), id); err != nil {
		return fiber.NewError(400, err.Error())
	}
	if err := t.table.Delete(ctx.Context(), id); err != nil {
		if err == db.ErrNotFound {
			return fiber.NewError(404, "not found")
		}
		return fiber.NewError(500, err.Error())
	}
	if err := t.hooks.AfterDelete(ctx.Context(), id); err != nil {
		logx.Errorf("crud: after delete hook error: %v", err)
	}
	return ctx.SendStatus(204)
}

// ---- MongoDB CRUDProvider ----

type mongoCRUD struct {
	model       *mon.Model
	lookupField string
}

// NewMongoCRUDProvider creates a CRUDProvider backed by MongoDB.
// lookupField is the document field used for Get/Update/Delete (e.g. "_id" or "short_code").
func NewMongoCRUDProvider(model *mon.Model, lookupField string) CRUDProvider {
	return &mongoCRUD{model: model, lookupField: lookupField}
}

func (m *mongoCRUD) List(ctx fiber.Ctx, params ListParams) error {
	var results []any
	if err := m.model.Find(ctx.Context(), &results, bson.M{}); err != nil {
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(results)
}

func (m *mongoCRUD) Get(ctx fiber.Ctx, id string) error {
	var result any
	filter := m.filterFor(id)
	if err := m.model.FindOne(ctx.Context(), &result, filter); err != nil {
		return fiber.NewError(404, "not found")
	}
	return ctx.JSON(result)
}

func (m *mongoCRUD) Create(ctx fiber.Ctx, body []byte) error {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	res, err := m.model.InsertOne(ctx.Context(), doc)
	if err != nil {
		return fiber.NewError(500, err.Error())
	}
	if m, ok := doc.(map[string]any); ok && res.InsertedID != nil {
		m["_id"] = res.InsertedID
	}
	return ctx.Status(201).JSON(doc)
}

func (m *mongoCRUD) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return fiber.NewError(400, fmt.Sprintf("invalid body: %v", err))
	}
	if _, err := m.model.UpdateOne(ctx.Context(), m.filterFor(id), bson.M{"$set": patch}); err != nil {
		return fiber.NewError(500, err.Error())
	}
	return ctx.JSON(patch)
}

func (m *mongoCRUD) Delete(ctx fiber.Ctx, id string) error {
	if _, err := m.model.DeleteOne(ctx.Context(), m.filterFor(id)); err != nil {
		return fiber.NewError(500, err.Error())
	}
	return ctx.SendStatus(204)
}

// filterFor converts the id string into a BSON filter, handling _id as ObjectID.
func (m *mongoCRUD) filterFor(id string) bson.M {
	if m.lookupField == "_id" {
		if oid, err := bson.ObjectIDFromHex(id); err == nil {
			return bson.M{"_id": oid}
		}
	}
	return bson.M{m.lookupField: id}
}
