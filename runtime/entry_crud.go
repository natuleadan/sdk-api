package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/events"
)

func registerCRUD(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker) error {
	provider, ok := handlers.CRUD[entry.Model]
	if !ok {
		return fmt.Errorf("crud model %q: no provider registered", entry.Model)
	}

	base := prefix + entry.Path
	ov := entry.Overrides
	if ov == nil {
		ov = &CRUDOverrides{}
	}

	ctx := context.Background()
	pubTargets := getPublishTargets(entry)
	hasPublish := len(pubTargets) > 0 && len(brokers) > 0

	if err := registerCRUDList(app, base, ov, handlers, provider); err != nil {
		return err
	}
	if err := registerCRUDGet(app, base, ov, handlers, provider); err != nil {
		return err
	}
	if err := registerCRUDCreate(app, base, ov, handlers, provider, ctx, pubTargets, entry, brokers, hasPublish); err != nil {
		return err
	}
	if err := registerCRUDUpdate(app, base, ov, handlers, provider, ctx, pubTargets, entry, brokers, hasPublish); err != nil {
		return err
	}
	if err := registerCRUDDelete(app, base, ov, handlers, provider, ctx, pubTargets, entry, brokers, hasPublish); err != nil {
		return err
	}

	return nil
}

func registerCRUDList(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider) error {
	if isDisabled(ov, ov.List) {
		return nil
	}
	if isOverridden(ov, ov.List) {
		h := resolveHandler(handlers.Rest, ov.List)
		if h == nil {
			return fmt.Errorf("crud list override: handler %q not found", ov.List)
		}
		app.Get(base, h)
	} else {
		app.Get(base, func(c fiber.Ctx) error {
			params := parseListParams(c)
			return provider.List(c, params)
		})
	}
	return nil
}

func registerCRUDGet(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider) error {
	if isDisabled(ov, ov.Get) {
		return nil
	}
	idParam := buildIDParam(base)
	if isOverridden(ov, ov.Get) {
		h := resolveHandler(handlers.Rest, ov.Get)
		if h == nil {
			return fmt.Errorf("crud get override: handler %q not found", ov.Get)
		}
		app.Get(base+idParam, h)
	} else {
		app.Get(base+idParam, func(c fiber.Ctx) error {
			return provider.Get(c, c.Params("id"))
		})
	}
	return nil
}

func registerCRUDCreate(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, ctx context.Context, pubTargets []EventPublishTarget, entry *EntryDef, brokers map[string]events.EventBroker, hasPublish bool) error {
	if isDisabled(ov, ov.Create) {
		return nil
	}
	var handler = resolveHandler(handlers.Rest, ov.Create)
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
	app.Post(base, handler)
	return nil
}

func registerCRUDUpdate(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, ctx context.Context, pubTargets []EventPublishTarget, entry *EntryDef, brokers map[string]events.EventBroker, hasPublish bool) error {
	if isDisabled(ov, ov.Update) {
		return nil
	}
	idParam := buildIDParam(base)
	var handler = resolveHandler(handlers.Rest, ov.Update)
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
	app.Patch(base+idParam, handler)
	return nil
}

func registerCRUDDelete(app *fiber.App, base string, ov *CRUDOverrides, handlers *EntryHandlers, provider CRUDProvider, ctx context.Context, pubTargets []EventPublishTarget, entry *EntryDef, brokers map[string]events.EventBroker, hasPublish bool) error {
	if isDisabled(ov, ov.Delete) {
		return nil
	}
	idParam := buildIDParam(base)
	var handler = resolveHandler(handlers.Rest, ov.Delete)
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
	app.Delete(base+idParam, handler)
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

func parseListParams(c fiber.Ctx) ListParams {
	page := fiber.Query[int](c, "page", 1)
	size := fiber.Query[int](c, "size", 10)
	sort := c.Query("sort", "id")

	filters := make(map[string]string)
	for key, value := range c.Request().URI().QueryArgs().All() {
		k := string(key)
		if k != "page" && k != "size" && k != "sort" {
			filters[k] = string(value)
		}
	}

	return ListParams{
		Page:    page,
		Size:    size,
		Sort:    sort,
		Filters: filters,
	}
}
