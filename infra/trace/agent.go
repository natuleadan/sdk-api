package trace

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/natuleadan/sdk-api/infra/logx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

const (
	kindZipkin   = "zipkin"
	kindOtlpGrpc = "otlpgrpc"
	kindOtlpHttp = "otlphttp"
	kindFile     = "file"
)

var (
	once           sync.Once
	tp             *sdktrace.TracerProvider
	shutdownOnceFn = sync.OnceFunc(func() {
		if tp != nil {
			if err := tp.Shutdown(context.Background()); err != nil {
		logx.Errorf("trace: shutdown error: %v", err)
	}
		}
	})
)

// StartAgent starts an opentelemetry agent.
// It uses sync.Once to ensure the agent is initialized only once,
// similar to prometheus.StartAgent and logx.SetUp.
// This prevents multiple ServiceConf.SetUp() calls from reinitializing
// the global tracer provider when running multiple servers (e.g., REST + RPC)
// in the same process.
func StartAgent(c Config) {
	if c.Disabled {
		return
	}

	once.Do(func() {
		if err := startAgent(c); err != nil {
			logx.Error(err)
		}
	})
}

// StopAgent shuts down the span processors in the order they were registered.
func StopAgent() {
	shutdownOnceFn()
}

func createExporter(c Config) (sdktrace.SpanExporter, error) {
	switch c.Batcher {
	case kindZipkin:
		// Migrated from deprecated zipkin exporter to OTLP HTTP.
		// Strip scheme and path to use bare host:port for OTLP.
		ep := strings.TrimPrefix(c.Endpoint, "http://")
		ep = strings.TrimPrefix(ep, "https://")
		if idx := strings.Index(ep, "/"); idx >= 0 {
			ep = ep[:idx]
		}
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(ep),
		}
		if len(c.OtlpHeaders) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(c.OtlpHeaders))
		}
		return otlptracehttp.New(context.Background(), opts...)
	case kindOtlpGrpc:
		// Always treat trace exporter as optional component, so we use nonblock here,
		// otherwise this would slow down app start up even set a dial timeout here when
		// endpoint can not reach.
		// If the connection not dial success, the global otel ErrorHandler will catch error
		// when reporting data like other exporters.
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithInsecure(),
			otlptracegrpc.WithEndpoint(c.Endpoint),
		}
		if len(c.OtlpHeaders) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(c.OtlpHeaders))
		}
		return otlptracegrpc.New(context.Background(), opts...)
	case kindOtlpHttp:
		// Not support flexible configuration now.
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpoint(c.Endpoint),
		}

		if !c.OtlpHttpSecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if len(c.OtlpHeaders) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(c.OtlpHeaders))
		}
		if len(c.OtlpHttpPath) > 0 {
			opts = append(opts, otlptracehttp.WithURLPath(c.OtlpHttpPath))
		}
		return otlptracehttp.New(context.Background(), opts...)
	case kindFile:
		f, err := os.OpenFile(c.Endpoint, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			return nil, fmt.Errorf("file exporter endpoint error: %s", err.Error())
		}
		return stdouttrace.New(stdouttrace.WithWriter(f))
	default:
		return nil, fmt.Errorf("unknown exporter: %s", c.Batcher)
	}
}

func startAgent(c Config) error {
	AddResources(semconv.ServiceNameKey.String(c.Name))

	opts := []sdktrace.TracerProviderOption{
		// Set the sampling rate based on the parent span to 100%
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.Sampler))),
		// Record information about this application in a Resource.
		sdktrace.WithResource(resource.NewSchemaless(attrResources...)),
	}

	if len(c.Endpoint) > 0 {
		exp, err := createExporter(c)
		if err != nil {
			logx.Error(err)
			return err
		}

		// Always be sure to batch in production.
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	tp = sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		logx.Errorf("[otel] error: %v", err)
	}))

	return nil
}
