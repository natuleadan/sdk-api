package runtime

import (
	"sync"

	"github.com/gofiber/fiber/v3"
)

// CRUDFactory creates a CRUDProvider when needed (lazy initialization).
// Use WithCRUDFactory instead of WithCRUD when the provider depends on
// resources initialized during Run() (e.g., database pools).
type CRUDFactory func() CRUDProvider

// lazyCRUD wraps a CRUDFactory and calls it once on first method invocation.
type lazyCRUD struct {
	factory  CRUDFactory
	provider CRUDProvider
	once     sync.Once
}

func (l *lazyCRUD) init() CRUDProvider {
	l.once.Do(func() {
		l.provider = l.factory()
		if l.provider == nil {
			panic("runtime: CRUDFactory returned nil provider")
		}
	})
	return l.provider
}

func (l *lazyCRUD) List(ctx fiber.Ctx, params ListParams) error { return l.init().List(ctx, params) }
func (l *lazyCRUD) Get(ctx fiber.Ctx, id string) error          { return l.init().Get(ctx, id) }
func (l *lazyCRUD) Create(ctx fiber.Ctx, body []byte) error     { return l.init().Create(ctx, body) }
func (l *lazyCRUD) Update(ctx fiber.Ctx, id string, body []byte) error {
	return l.init().Update(ctx, id, body)
}
func (l *lazyCRUD) Delete(ctx fiber.Ctx, id string) error { return l.init().Delete(ctx, id) }
