package runtime

import (
	"context"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"github.com/natuleadan/sdk-api/events"
	"github.com/natuleadan/sdk-api/infra/logx"
)

func registerREST(app *fiber.App, entry *EntryDef, handlers *EntryHandlers, prefix string, brokers map[string]events.EventBroker) error {
	h := resolveHandler(handlers.Rest, entry.Handler)
	if h == nil {
		return fmt.Errorf("rest handler %q not found", entry.Handler)
	}
	targets := getPublishTargets(entry)
	if len(targets) > 0 && len(brokers) > 0 {
		h = wrapEventPublish(context.Background(), h, targets, entry.EventStream, brokers)
	}
	path := prefix + entry.Path
	switch entry.Method {
	case "GET":
		app.Get(path, h)
	case "POST":
		app.Post(path, h)
	case "PUT":
		app.Put(path, h)
	case "PATCH":
		app.Patch(path, h)
	case "DELETE":
		app.Delete(path, h)
	default:
		return fmt.Errorf("unsupported HTTP method %q", entry.Method)
	}
	return nil
}

func getPublishTargets(entry *EntryDef) []EventPublishTarget {
	if len(entry.EventPublish) > 0 {
		return entry.EventPublish
	}
	return entry.NATSPublish
}

func wrapEventPublish(ctx context.Context, handler func(*fiber.Ctx) error, targets []EventPublishTarget, entryEventStream string, brokers map[string]events.EventBroker) func(*fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		err := handler(c)
		if err == nil && c.Response().StatusCode() < 400 {
			for _, target := range targets {
				subject := target.Subject
				if subject == "" {
					subject = target.Stream
				}
				targetBrokers := selectBrokers(target, entryEventStream, brokers)
				for _, broker := range targetBrokers {
					if pubErr := broker.Publish(ctx, subject, c.Body()); pubErr != nil {
						logx.Errorf("event_publish %s: %v", subject, pubErr)
					}
				}
			}
		}
		return err
	}
}

func selectBrokers(target EventPublishTarget, entryEventStream string, allBrokers map[string]events.EventBroker) map[string]events.EventBroker {
	brokerName := target.EventStream
	if brokerName == "" {
		brokerName = entryEventStream
	}
	if brokerName != "" {
		if b, ok := allBrokers[brokerName]; ok {
			return map[string]events.EventBroker{brokerName: b}
		}
	}
	return allBrokers
}
