package runtime

import (
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/graphql-go/graphql"
	"github.com/natuleadan/sdk-api/db"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func registerGraphQL(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, models map[string]*db.TableInfo) error {
	path := prefix + entry.Path

	schema, err := buildGraphQLSchema(handlers, models)
	if err != nil {
		return err
	}

	app.Post(path, func(c *fiber.Ctx) error {
		var req struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables,omitempty"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
		}
		if req.Query == "" {
			return c.Status(400).JSON(fiber.Map{"error": "query is required"})
		}

		result := graphql.Do(graphql.Params{
			Schema:         *schema,
			RequestString:  req.Query,
			VariableValues: req.Variables,
		})

		if len(result.Errors) > 0 {
			logx.Errorf("graphql query errors: %v", result.Errors)
		}
		return c.Status(http.StatusOK).JSON(result)
	})

	return nil
}
