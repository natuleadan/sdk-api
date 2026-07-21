package runtime

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/runtime/errcode"
)

type cachedMarker interface{ isCached() }

func registerCRUD(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker, mws []fiber.Handler) error {
	provider, ok := handlers.CRUD[entry.Model]
	if !ok {
		return fmt.Errorf("crud model %q: no provider registered", entry.Model)
	}

	if entry.Cache != "" {
		if _, ok := provider.(cachedMarker); !ok {
			return fmt.Errorf("crud model %q: cache=%q set in YAML but provider is not cached. Use runtime.CachedCRUD (or MySQLCachedCRUD) instead of MustRegister", entry.Model, entry.Cache)
		}
	}

	base := prefix + entry.Path
	ov := entry.Overrides
	if ov == nil {
		ov = &CRUDOverrides{}
	}

	// Tenant scoping middleware: extracts tenant ID from the JWT claim configured in tenant_scope
	if entry.TenantField != "" {
		mws = append(mws, buildTenantMiddleware(entry))
	}

	ctx := context.Background()
	pubTargets := getPublishTargets(entry)
	hasPublish := len(pubTargets) > 0 && len(brokers) > 0

	if err := registerCRUDList(app, base, ov, handlers, provider, entry, mws); err != nil {
		return err
	}
	if err := registerCRUDGet(app, base, ov, handlers, provider, mws); err != nil {
		return err
	}
	if err := registerCRUDCreate(app, base, ov, handlers, provider, ctx, pubTargets, entry, brokers, hasPublish, mws); err != nil {
		return err
	}
	if err := registerCRUDUpdate(app, base, ov, handlers, provider, ctx, pubTargets, entry, brokers, hasPublish, mws); err != nil {
		return err
	}
	if err := registerCRUDDelete(app, base, ov, handlers, provider, ctx, pubTargets, entry, brokers, hasPublish, mws); err != nil {
		return err
	}
	return nil
}

func buildTenantMiddleware(entry *EntryDef) fiber.Handler {
	scope := entry.TenantScope
	if scope == "" {
		scope = "org_id"
	}
	return func(c fiber.Ctx) error {
		tenantID := ""
		if claims, ok := c.Locals("claims").(jwt.MapClaims); ok {
			if tid, ok := claims[scope].(string); ok {
				tenantID = tid
			}
		}
		if tenantID == "" {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code":    403,
				"message": fmt.Sprintf("tenant scope %q not found in token claims", scope),
			})
		}
		c.Locals("tenant_field", entry.TenantField)
		c.Locals("tenant_id", tenantID)
		return c.Next()
	}
}

func registerCRUDList(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, entry *EntryDef, mws []fiber.Handler) error {
	if isDisabled(ov, ov.List) {
		return nil
	}
	if isOverridden(ov, ov.List) {
		h := resolveHandler(handlers.Rest, ov.List)
		if h == nil {
			return fmt.Errorf("crud list override: handler %q not found", ov.List)
		}
		registerWithMws(app, "GET", base, mws, h)
	} else {
		pageSize := entry.PageSize
		if pageSize < 1 {
			pageSize = 10
		}
		maxPageSize := entry.MaxPageSize
		if maxPageSize < 1 {
			maxPageSize = 100
		}
		if maxPageSize < pageSize {
			maxPageSize = pageSize
		}
		pagination := entry.Pagination
		if pagination == "" {
			pagination = "offset"
		}
		sortable := entry.Sortable

		registerWithMws(app, "GET", base, mws, func(c fiber.Ctx) error {
			params, err := parseListParams(c, pageSize, maxPageSize, sortable, pagination)
			if err != nil {
				return err
			}
			return provider.List(c, params)
		})
	}
	return nil
}

func registerCRUDGet(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, mws []fiber.Handler) error {
	if isDisabled(ov, ov.Get) {
		return nil
	}
	idParam := buildIDParam(base)
	if isOverridden(ov, ov.Get) {
		h := resolveHandler(handlers.Rest, ov.Get)
		if h == nil {
			return fmt.Errorf("crud get override: handler %q not found", ov.Get)
		}
		registerWithMws(app, "GET", base+idParam, mws, h)
	} else {
		registerWithMws(app, "GET", base+idParam, mws, func(c fiber.Ctx) error {
			return provider.Get(c, c.Params("id"))
		})
	}
	return nil
}

func registerCRUDCreate(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, ctx context.Context, pubTargets []EventPublishTarget, entry *EntryDef, brokers map[string]events.EventBroker, hasPublish bool, mws []fiber.Handler) error {
	if isDisabled(ov, ov.Create) {
		return nil
	}
	handler := resolveHandler(handlers.Rest, ov.Create)
	if isOverridden(ov, ov.Create) {
		if handler == nil {
			return fmt.Errorf("crud create override: handler %q not found", ov.Create)
		}
	} else {
		handler = func(c fiber.Ctx) error {
			return provider.Create(c, c.Body())
		}
	}
	if hasPublish {
		handler = wrapEventPublish(ctx, handler, pubTargets, entry.EventStream, brokers)
	}
	registerWithMws(app, "POST", base, mws, handler)
	return nil
}

func registerCRUDUpdate(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, ctx context.Context, pubTargets []EventPublishTarget, entry *EntryDef, brokers map[string]events.EventBroker, hasPublish bool, mws []fiber.Handler) error {
	if isDisabled(ov, ov.Update) {
		return nil
	}
	idParam := buildIDParam(base)
	handler := resolveHandler(handlers.Rest, ov.Update)
	if isOverridden(ov, ov.Update) {
		if handler == nil {
			return fmt.Errorf("crud update override: handler %q not found", ov.Update)
		}
	} else {
		handler = func(c fiber.Ctx) error {
			return provider.Update(c, c.Params("id"), c.Body())
		}
	}
	if hasPublish {
		handler = wrapEventPublish(ctx, handler, pubTargets, entry.EventStream, brokers)
	}
	registerWithMws(app, "PATCH", base+idParam, mws, handler)
	return nil
}

func registerCRUDDelete(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, ctx context.Context, pubTargets []EventPublishTarget, entry *EntryDef, brokers map[string]events.EventBroker, hasPublish bool, mws []fiber.Handler) error {
	if isDisabled(ov, ov.Delete) {
		return nil
	}
	idParam := buildIDParam(base)
	handler := resolveHandler(handlers.Rest, ov.Delete)
	if isOverridden(ov, ov.Delete) {
		if handler == nil {
			return fmt.Errorf("crud delete override: handler %q not found", ov.Delete)
		}
	} else {
		handler = func(c fiber.Ctx) error {
			return provider.Delete(c, c.Params("id"))
		}
	}
	if hasPublish {
		handler = wrapEventPublish(ctx, handler, pubTargets, entry.EventStream, brokers)
	}
	registerWithMws(app, "DELETE", base+idParam, mws, handler)
	return nil
}

func isDisabled(ov *CRUDOverrides, field string) bool {
	if ov == nil {
		return false
	}
	return field == "-"
}

func isOverridden(ov *CRUDOverrides, field string) bool {
	if ov == nil {
		return false
	}
	return field != "" && field != "-"
}

func resolveHandler(m map[string]func(fiber.Ctx) error, name string) func(fiber.Ctx) error {
	if m == nil {
		return nil
	}
	return m[name]
}

func buildIDParam(path string) string {
	for part := range strings.SplitSeq(path, "/") {
		if strings.HasPrefix(part, ":") {
			return ""
		}
	}
	return "/:id"
}

func parseListParams(c fiber.Ctx, pageSize, maxPageSize int, sortable []string, pagination string) (ListParams, error) {
	size := min(max(fiber.Query[int](c, "size", pageSize), pageSize), maxPageSize)

	sort := c.Query("sort", "id")
	if sort == "" {
		sort = "id"
	}
	if len(sortable) > 0 && !slices.Contains(sortable, sort) {
		return ListParams{}, errcode.ErrValidation("sort", "invalid", sort)
	}

	page := max(fiber.Query[int](c, "page", 1), 1)

	cursor := c.Query("cursor", "")

	filters := make(map[string]string)
	for key, value := range c.Request().URI().QueryArgs().All() {
		k := string(key)
		if k != "page" && k != "size" && k != "sort" && k != "cursor" {
			filters[k] = string(value)
		}
	}

	return ListParams{
		Page:       page,
		Size:       size,
		Sort:       sort,
		Filters:    filters,
		Cursor:     cursor,
		Pagination: pagination,
	}, nil
}
