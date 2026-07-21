package runtime

import (
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v3"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/stores/mon"
	"github.com/natuleadan/sdk-api/runtime/errcode"
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

func tenantInfo(c fiber.Ctx) (field, id string) {
	f, okF := c.Locals("tenant_field").(string)
	i, okI := c.Locals("tenant_id").(string)
	if okF && okI && f != "" && i != "" {
		return f, i
	}
	return "", ""
}

func (t *tableCRUD[T]) List(ctx fiber.Ctx, params ListParams) error {
	tf, tid := tenantInfo(ctx)
	if params.Pagination == "keyset" {
		where := makeFiltersMap(params.Filters)
		if tf != "" && tid != "" {
			if where == nil {
				where = make(map[string]any)
			}
			where[tf] = tid
		}
		items, nextCursor, err := t.table.QueryKeyset(ctx.Context(), params.Cursor, params.Size, params.Sort, where)
		if err != nil {
			return errcode.ErrDBQuery("QueryKeyset", "table", err)
		}
		return ctx.JSON(KeysetResponse{Data: items, NextCursor: nextCursor, PageSize: params.Size})
	}

	if len(params.Filters) > 0 || (tf != "" && tid != "") {
		where := make(map[string]any, len(params.Filters)+1)
		var wcs []db.ColumnValue
		for k, v := range params.Filters {
			where[k] = v
			wcs = append(wcs, db.Col(k, v))
		}
		if tf != "" && tid != "" {
			where[tf] = tid
			wcs = append(wcs, db.Col(tf, tid))
		}
		items, err := t.table.QueryWhere(ctx.Context(), where, params.Sort, params.Size, (params.Page-1)*params.Size)
		if err != nil {
			return errcode.ErrDBQuery("QueryWhere", "table", err)
		}
		total, err := t.table.Count(ctx.Context(), wcs...)
		if err != nil {
			return errcode.ErrDBQuery("Count", "table", err)
		}
		return ctx.JSON(PaginatedResponse{Data: items, Total: total, Page: params.Page, Size: params.Size})
	}

	items, total, err := t.table.QueryPaginated(ctx.Context(), params.Page, params.Size, params.Sort)
	if err != nil {
		return errcode.ErrDBQuery("QueryPaginated", "table", err)
	}
	return ctx.JSON(PaginatedResponse{Data: items, Total: total, Page: params.Page, Size: params.Size})
}

func (t *tableCRUD[T]) Get(ctx fiber.Ctx, id string) error {
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		item, err := t.table.GetScoped(ctx.Context(), id, tf, tid)
		if err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
		return ctx.JSON(item)
	}
	item, err := t.table.Get(ctx.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return errcode.ErrNotFound("record", id)
		}
		return errcode.ErrDBQuery("op", "table", err)
	}
	return ctx.JSON(item)
}

func (t *tableCRUD[T]) Create(ctx fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}

	var err error
	entity, err = t.hooks.BeforeCreate(ctx.Context(), entity)
	if err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}

	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		if err := t.table.CreateScoped(ctx.Context(), &entity, tf, tid); err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
	} else {
		if err := t.table.Create(ctx.Context(), &entity); err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
	}

	if err := t.hooks.AfterCreate(ctx.Context(), &entity); err != nil {
		logx.Errorf("after create hook: %v", err)
	}
	return ctx.Status(201).JSON(entity)
}

func (t *tableCRUD[T]) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}

	var err error
	patch, err = t.hooks.BeforeUpdate(ctx.Context(), id, patch)
	if err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}

	patch = t.table.ResolvePatch(patch)
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		entity, err := t.table.UpdateScoped(ctx.Context(), id, patch, tf, tid)
		if err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
		if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
			logx.Errorf("after update hook: %v", err)
		}
		return ctx.JSON(entity)
	}

	entity, err := t.table.Update(ctx.Context(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return errcode.ErrNotFound("record", id)
		}
		return errcode.ErrDBQuery("op", "table", err)
	}

	if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
		logx.Errorf("after update hook: %v", err)
	}
	return ctx.JSON(entity)
}

func (t *tableCRUD[T]) Delete(ctx fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.Context(), id); err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		if err := t.table.DeleteScoped(ctx.Context(), id, tf, tid); err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
	} else {
		if err := t.table.Delete(ctx.Context(), id); err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
	}
	if err := t.hooks.AfterDelete(ctx.Context(), id); err != nil {
		logx.Errorf("after delete hook: %v", err)
	}
	return ctx.SendStatus(204)
}

// PaginatedResponse is used by tableCRUD.List (offset mode).
type PaginatedResponse struct {
	Data  any   `json:"data"`
	Total int64 `json:"total"`
	Page  int   `json:"page"`
	Size  int   `json:"size"`
}

// KeysetResponse is used by tableCRUD.List (keyset mode).
type KeysetResponse struct {
	Data       any    `json:"data"`
	NextCursor string `json:"nextCursor,omitempty"`
	PageSize   int    `json:"pageSize"`
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
	tf, tid := tenantInfo(ctx)
	if params.Pagination == "keyset" {
		where := makeFiltersMap(params.Filters)
		if tf != "" && tid != "" {
			if where == nil {
				where = make(map[string]any)
			}
			where[tf] = tid
		}
		items, nextCursor, err := t.table.QueryKeyset(ctx.Context(), params.Cursor, params.Size, params.Sort, where)
		if err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
		return ctx.JSON(KeysetResponse{Data: items, NextCursor: nextCursor, PageSize: params.Size})
	}
	var items []T
	var err error
	if tf != "" && tid != "" {
		items, err = t.table.ListScoped(ctx.Context(), tf, tid)
	} else {
		items, err = t.table.List(ctx.Context())
	}
	if err != nil {
		return errcode.ErrDBQuery("op", "table", err)
	}
	total := int64(len(items))
	start := min(max((params.Page-1)*params.Size, 0), len(items))
	end := min(start+params.Size, len(items))
	return ctx.JSON(PaginatedResponse{Data: items[start:end], Total: total, Page: params.Page, Size: params.Size})
}

func (t *mysqlCRUD[T]) Get(ctx fiber.Ctx, id string) error {
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		item, err := t.table.GetScoped(ctx.Context(), id, tf, tid)
		if err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
		return ctx.JSON(item)
	}
	item, err := t.table.Get(ctx.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return errcode.ErrNotFound("record", id)
		}
		return errcode.ErrDBQuery("op", "table", err)
	}
	return ctx.JSON(item)
}

func (t *mysqlCRUD[T]) Create(ctx fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}
	entity, err := t.hooks.BeforeCreate(ctx.Context(), entity)
	if err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		if err := t.table.CreateScoped(ctx.Context(), &entity, tf, tid); err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
	} else {
		if err := t.table.Create(ctx.Context(), &entity); err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
	}
	if err := t.hooks.AfterCreate(ctx.Context(), &entity); err != nil {
		logx.Errorf("crud: after create hook error: %v", err)
	}
	return ctx.Status(201).JSON(entity)
}

func (t *mysqlCRUD[T]) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}
	patch, err := t.hooks.BeforeUpdate(ctx.Context(), id, patch)
	if err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		entity, err := t.table.UpdateScoped(ctx.Context(), id, patch, tf, tid)
		if err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
		if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
			logx.Errorf("crud: after update hook error: %v", err)
		}
		return ctx.JSON(entity)
	}
	entity, err := t.table.Update(ctx.Context(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return errcode.ErrNotFound("record", id)
		}
		return errcode.ErrDBQuery("op", "table", err)
	}
	if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
		logx.Errorf("crud: after update hook error: %v", err)
	}
	return ctx.JSON(entity)
}

func (t *mysqlCRUD[T]) Delete(ctx fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.Context(), id); err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		if err := t.table.DeleteScoped(ctx.Context(), id, tf, tid); err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
	} else {
		if err := t.table.Delete(ctx.Context(), id); err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
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
	tf, tid := tenantInfo(ctx)
	if params.Pagination == "keyset" {
		where := makeFiltersMap(params.Filters)
		if tf != "" && tid != "" {
			if where == nil {
				where = make(map[string]any)
			}
			where[tf] = tid
		}
		items, nextCursor, err := t.table.QueryKeyset(ctx.Context(), params.Cursor, params.Size, params.Sort, where)
		if err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
		return ctx.JSON(KeysetResponse{Data: items, NextCursor: nextCursor, PageSize: params.Size})
	}
	var items []T
	var err error
	if tf != "" && tid != "" {
		items, err = t.table.ListScoped(ctx.Context(), tf, tid)
	} else {
		items, err = t.table.List(ctx.Context())
	}
	if err != nil {
		return errcode.ErrDBQuery("op", "table", err)
	}
	total := int64(len(items))
	start := min(max((params.Page-1)*params.Size, 0), len(items))
	end := min(start+params.Size, len(items))
	return ctx.JSON(PaginatedResponse{Data: items[start:end], Total: total, Page: params.Page, Size: params.Size})
}

func (t *tursoCRUD[T]) Get(ctx fiber.Ctx, id string) error {
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		item, err := t.table.GetScoped(ctx.Context(), id, tf, tid)
		if err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
		return ctx.JSON(item)
	}
	item, err := t.table.Get(ctx.Context(), id)
	if err != nil {
		if err == db.ErrNotFound {
			return errcode.ErrNotFound("record", id)
		}
		return errcode.ErrDBQuery("op", "table", err)
	}
	return ctx.JSON(item)
}

func (t *tursoCRUD[T]) Create(ctx fiber.Ctx, body []byte) error {
	var entity T
	if err := json.Unmarshal(body, &entity); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}
	entity, err := t.hooks.BeforeCreate(ctx.Context(), entity)
	if err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		if err := t.table.CreateScoped(ctx.Context(), &entity, tf, tid); err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
	} else {
		if err := t.table.Create(ctx.Context(), &entity); err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
	}
	if err := t.hooks.AfterCreate(ctx.Context(), &entity); err != nil {
		logx.Errorf("crud: after create hook error: %v", err)
	}
	return ctx.Status(201).JSON(entity)
}

func (t *tursoCRUD[T]) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}
	patch, err := t.hooks.BeforeUpdate(ctx.Context(), id, patch)
	if err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		entity, err := t.table.UpdateScoped(ctx.Context(), id, patch, tf, tid)
		if err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
		if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
			logx.Errorf("crud: after update hook error: %v", err)
		}
		return ctx.JSON(entity)
	}
	entity, err := t.table.Update(ctx.Context(), id, patch)
	if err != nil {
		if err == db.ErrNotFound {
			return errcode.ErrNotFound("record", id)
		}
		return errcode.ErrDBQuery("op", "table", err)
	}
	if err := t.hooks.AfterUpdate(ctx.Context(), entity); err != nil {
		logx.Errorf("crud: after update hook error: %v", err)
	}
	return ctx.JSON(entity)
}

func (t *tursoCRUD[T]) Delete(ctx fiber.Ctx, id string) error {
	if err := t.hooks.BeforeDelete(ctx.Context(), id); err != nil {
		return errcode.ErrValidation("hook", "rejected", err)
	}
	tf, tid := tenantInfo(ctx)
	if tf != "" && tid != "" {
		if err := t.table.DeleteScoped(ctx.Context(), id, tf, tid); err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
	} else {
		if err := t.table.Delete(ctx.Context(), id); err != nil {
			if err == db.ErrNotFound {
				return errcode.ErrNotFound("record", id)
			}
			return errcode.ErrDBQuery("op", "table", err)
		}
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
	tf, tid := tenantInfo(ctx)
	filter := bson.M{}
	if tf != "" && tid != "" {
		filter[tf] = tid
	}

	if params.Pagination == "keyset" {
		size := params.Size + 1
		findOpts := options.Find().SetLimit(int64(size)).SetSort(bson.D{{Key: "_id", Value: 1}})
		if params.Cursor != "" {
			oid, err := bson.ObjectIDFromHex(params.Cursor)
			if err != nil {
				return errcode.ErrValidation("cursor", "invalid", params.Cursor)
			}
			filter["_id"] = bson.M{"$gt": oid}
		}
		var results []bson.M
		if err := m.model.Find(ctx.Context(), &results, filter, findOpts); err != nil {
			return errcode.ErrDBQuery("op", "table", err)
		}
		nextCursor := ""
		if len(results) > params.Size {
			if id, ok := results[params.Size-1]["_id"].(bson.ObjectID); ok {
				nextCursor = id.Hex()
			}
			results = results[:params.Size]
		}
		return ctx.JSON(KeysetResponse{Data: results, NextCursor: nextCursor, PageSize: params.Size})
	}

	size := max(params.Size, 1)
	skip := max((params.Page-1)*size, 0)
	findOpts := options.Find().SetSkip(int64(skip)).SetLimit(int64(size)).SetSort(bson.D{{Key: "_id", Value: 1}})
	var results []bson.M
	if err := m.model.Find(ctx.Context(), &results, filter, findOpts); err != nil {
		return errcode.ErrDBQuery("op", "table", err)
	}
	return ctx.JSON(PaginatedResponse{Data: results, Total: 0, Page: params.Page, Size: size})
}

func (m *mongoCRUD) Get(ctx fiber.Ctx, id string) error {
	var result any
	filter := m.filterFor(id)
	if tf, tid := tenantInfo(ctx); tf != "" && tid != "" {
		filter[tf] = tid
	}
	if err := m.model.FindOne(ctx.Context(), &result, filter); err != nil {
		return errcode.ErrNotFound("record", id)
	}
	return ctx.JSON(result)
}

func (m *mongoCRUD) Create(ctx fiber.Ctx, body []byte) error {
	var doc any
	if err := json.Unmarshal(body, &doc); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}
	if tf, tid := tenantInfo(ctx); tf != "" && tid != "" {
		if m, ok := doc.(map[string]any); ok {
			m[tf] = tid
		}
	}
	res, err := m.model.InsertOne(ctx.Context(), doc)
	if err != nil {
		return errcode.ErrDBQuery("op", "table", err)
	}
	if m, ok := doc.(map[string]any); ok && res.InsertedID != nil {
		m["_id"] = res.InsertedID
	}
	return ctx.Status(201).JSON(doc)
}

func (m *mongoCRUD) Update(ctx fiber.Ctx, id string, body []byte) error {
	var patch map[string]any
	if err := json.Unmarshal(body, &patch); err != nil {
		return errcode.ErrValidation("body", "json", err)
	}
	filter := m.filterFor(id)
	if tf, tid := tenantInfo(ctx); tf != "" && tid != "" {
		filter[tf] = tid
	}
	if _, err := m.model.UpdateOne(ctx.Context(), filter, bson.M{"$set": patch}); err != nil {
		return errcode.ErrDBQuery("op", "table", err)
	}
	return ctx.JSON(patch)
}

func (m *mongoCRUD) Delete(ctx fiber.Ctx, id string) error {
	filter := m.filterFor(id)
	if tf, tid := tenantInfo(ctx); tf != "" && tid != "" {
		filter[tf] = tid
	}
	if _, err := m.model.DeleteOne(ctx.Context(), filter); err != nil {
		return errcode.ErrDBQuery("op", "table", err)
	}
	return ctx.SendStatus(204)
}

// makeFiltersMap converts ListParams.Filters (map[string]string) to map[string]any.
func makeFiltersMap(filters map[string]string) map[string]any {
	if len(filters) == 0 {
		return nil
	}
	m := make(map[string]any, len(filters))
	for k, v := range filters {
		m[k] = v
	}
	return m
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
