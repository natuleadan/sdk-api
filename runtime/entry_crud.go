package runtime

import (
	"fmt"
	"strings"

	"github.com/gofiber/fiber/v2"
)

func registerCRUD(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string) error {
	provider, ok := handlers.CRUD[entry.Model]
	if !ok {
		return fmt.Errorf("crud model %q: no provider registered", entry.Model)
	}

	base := prefix + entry.Path
	ov := entry.Overrides
	if ov == nil {
		ov = &CRUDOverrides{}
	}

	// GET /resource — list
	if !isDisabled(ov, ov.List) {
		if isOverridden(ov, ov.List) {
			h := resolveHandler(handlers.Rest, ov.List)
			if h == nil {
				return fmt.Errorf("crud list override: handler %q not found", ov.List)
			}
			app.Get(base, h)
		} else {
			app.Get(base, func(c *fiber.Ctx) error {
				params := parseListParams(c)
				if err := provider.List(c, params); err != nil {
					return err
				}
				return nil
			})
		}
	}

	// GET /resource/:id — get one
	if !isDisabled(ov, ov.Get) {
		idParam := buildIDParam(entry.Path)
		if isOverridden(ov, ov.Get) {
			h := resolveHandler(handlers.Rest, ov.Get)
			if h == nil {
				return fmt.Errorf("crud get override: handler %q not found", ov.Get)
			}
			app.Get(base+idParam, h)
		} else {
			app.Get(base+idParam, func(c *fiber.Ctx) error {
				id := c.Params("id")
				if err := provider.Get(c, id); err != nil {
					return err
				}
				return nil
			})
		}
	}

	// POST /resource — create
	if !isDisabled(ov, ov.Create) {
		if isOverridden(ov, ov.Create) {
			h := resolveHandler(handlers.Rest, ov.Create)
			if h == nil {
				return fmt.Errorf("crud create override: handler %q not found", ov.Create)
			}
			app.Post(base, h)
		} else {
			app.Post(base, func(c *fiber.Ctx) error {
				if err := provider.Create(c, c.Body()); err != nil {
					return err
				}
				return nil
			})
		}
	}

	// PATCH /resource/:id — update
	if !isDisabled(ov, ov.Update) {
		idParam := buildIDParam(entry.Path)
		if isOverridden(ov, ov.Update) {
			h := resolveHandler(handlers.Rest, ov.Update)
			if h == nil {
				return fmt.Errorf("crud update override: handler %q not found", ov.Update)
			}
			app.Patch(base+idParam, h)
		} else {
			app.Patch(base+idParam, func(c *fiber.Ctx) error {
				id := c.Params("id")
				if err := provider.Update(c, id, c.Body()); err != nil {
					return err
				}
				return nil
			})
		}
	}

	// DELETE /resource/:id — delete
	if !isDisabled(ov, ov.Delete) {
		idParam := buildIDParam(entry.Path)
		if isOverridden(ov, ov.Delete) {
			h := resolveHandler(handlers.Rest, ov.Delete)
			if h == nil {
				return fmt.Errorf("crud delete override: handler %q not found", ov.Delete)
			}
			app.Delete(base+idParam, h)
		} else {
			app.Delete(base+idParam, func(c *fiber.Ctx) error {
				id := c.Params("id")
				if err := provider.Delete(c, id); err != nil {
					return err
				}
				return nil
			})
		}
	}

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

func resolveHandler(m map[string]func(*fiber.Ctx) error, name string) func(*fiber.Ctx) error {
	if m == nil {
		return nil
	}
	return m[name]
}

func buildIDParam(path string) string {
	for _, part := range strings.Split(path, "/") {
		if strings.HasPrefix(part, ":") {
			return ""
		}
	}
	return "/:id"
}

func parseListParams(c *fiber.Ctx) ListParams {
	page := c.QueryInt("page", 1)
	size := c.QueryInt("size", 10)
	sort := c.Query("sort", "id")

	filters := make(map[string]string)
	c.Request().URI().QueryArgs().VisitAll(func(key, value []byte) {
		k := string(key)
		if k != "page" && k != "size" && k != "sort" {
			filters[k] = string(value)
		}
	})

	return ListParams{
		Page:    page,
		Size:    size,
		Sort:    sort,
		Filters: filters,
	}
}
