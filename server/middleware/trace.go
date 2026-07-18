package middleware

import (
	"github.com/gofiber/fiber/v3"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/trace"
	"github.com/valyala/fasthttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

type requestCarrier struct {
	req *fasthttp.Request
}

func (c *requestCarrier) Get(key string) string {
	return string(c.req.Header.Peek(key))
}

func (c *requestCarrier) Set(key, value string) {
	c.req.Header.Set(key, value)
}

func (c *requestCarrier) Keys() []string {
	var keys []string
	for k := range c.req.Header.All() {
		keys = append(keys, string(k))
	}
	return keys
}

type responseCarrier struct {
	resp *fasthttp.Response
}

func (c *responseCarrier) Get(key string) string {
	return string(c.resp.Header.Peek(key))
}

func (c *responseCarrier) Set(key, value string) {
	c.resp.Header.Set(key, value)
}

func (c *responseCarrier) Keys() []string {
	var keys []string
	for k := range c.resp.Header.All() {
		keys = append(keys, string(k))
	}
	return keys
}

type TraceConfig struct {
	Name     string
	Endpoint string
	Sampler  float64
	Batcher  string

	OtlpHeaders    map[string]string
	OtlpHttpPath   string
	OtlpHttpSecure bool

	TraceResponseHeader string
	CustomAttributes    func(fiber.Ctx) []attribute.KeyValue
	SkipPaths           []string
}

func Trace(cfg TraceConfig) fiber.Handler {
	batcher := cfg.Batcher
	if batcher == "" {
		batcher = "otlpgrpc"
	}
	trace.StartAgent(trace.Config{
		Name:           cfg.Name,
		Endpoint:       cfg.Endpoint,
		Sampler:        cfg.Sampler,
		Batcher:        batcher,
		OtlpHeaders:    cfg.OtlpHeaders,
		OtlpHttpPath:   cfg.OtlpHttpPath,
		OtlpHttpSecure: cfg.OtlpHttpSecure,
	})

	tracer := otel.Tracer(trace.TraceName)
	propagator := otel.GetTextMapPropagator()

	skipSet := make(map[string]struct{}, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = struct{}{}
	}

	return func(c fiber.Ctx) error {
		path := string(c.Request().URI().Path())
		if _, skip := skipSet[path]; skip {
			return c.Next()
		}

		ctx := propagator.Extract(c.Context(), &requestCarrier{req: c.Request()})

		opts := []oteltrace.SpanStartOption{
			oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			oteltrace.WithAttributes(
				semconv.HTTPMethodKey.String(c.Method()),
				semconv.HTTPTargetKey.String(path),
				semconv.HTTPRouteKey.String(c.Route().Path),
				semconv.HTTPClientIPKey.String(c.IP()),
			),
		}
		if cfg.CustomAttributes != nil {
			opts = append(opts, oteltrace.WithAttributes(cfg.CustomAttributes(c)...))
		}

		spanCtx, span := tracer.Start(ctx, path, opts...)
		defer span.End()

		if cfg.TraceResponseHeader != "" {
			c.Set(cfg.TraceResponseHeader, span.SpanContext().TraceID().String())
		}

		propagator.Inject(spanCtx, &responseCarrier{resp: c.Response()})
		c.SetContext(spanCtx)

		err := c.Next()

		span.SetAttributes(semconv.HTTPStatusCodeKey.Int(c.Response().StatusCode()))
		if err != nil {
			logx.Errorf("trace: request error: %v", err)
			span.SetAttributes(semconv.HTTPStatusCodeKey.Int(c.Response().StatusCode()))
			span.RecordError(err)
		}

		return err
	}
}
