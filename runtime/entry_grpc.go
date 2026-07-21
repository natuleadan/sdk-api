package runtime

import (
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func registerGRPC(app *fiber.App, entry *EntryDef, _ *EntryHandlers, prefix string, _ map[string]events.EventBroker, _ map[string]*db.TableInfo, _ []fiber.Handler) error { //nolint:unparam
	logx.Infof("gRPC entry registered: service=%s", entry.ServiceName)

	app.Get(prefix+entry.Path, func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"service": entry.ServiceName,
			"type":    "grpc",
		})
	})

	return nil
}
