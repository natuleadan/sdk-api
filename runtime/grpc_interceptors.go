package runtime

import (
	"context"
	"time"

	"github.com/natuleadan/sdk-api/infra/breaker"
	"github.com/natuleadan/sdk-api/infra/load"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/trace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	oteltrace "go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type GrpcInterceptorsConfig struct {
	Trace    bool
	Breaker  bool
	Timeout  bool
	Shedding bool
}

func defaultGrpcInterceptorsConfig() GrpcInterceptorsConfig {
	return GrpcInterceptorsConfig{
		Trace:    true,
		Breaker:  true,
		Timeout:  true,
		Shedding: true,
	}
}

// --- Server Unary Interceptors ---

func unaryTracingInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	propagator := otel.GetTextMapPropagator()
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		ctx = propagator.Extract(ctx, &metadataCarrier{md: md})
	}

	tracer := otel.Tracer(trace.TraceName)
	spanCtx, span := tracer.Start(ctx, info.FullMethod,
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		oteltrace.WithAttributes(
			semconv.RPCServiceKey.String(info.FullMethod),
		),
	)
	defer span.End()

	resp, err := handler(spanCtx, req)
	if err != nil {
		span.RecordError(err)
		span.SetAttributes(attribute.String("error", err.Error()))
	}
	return resp, err
}

func unaryBreakerInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	b := breaker.GetBreaker(info.FullMethod)
	var resp any
	err := b.DoWithAcceptable(func() error {
		var innerErr error
		resp, innerErr = handler(ctx, req)
		return innerErr
	}, func(err error) bool {
		return err == nil || status.Code(err) != codes.Internal
	})
	if err == breaker.ErrServiceUnavailable {
		return nil, status.Error(codes.Unavailable, "service overloaded")
	}
	return resp, err
}

func unaryTimeoutInterceptor(timeout time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if timeout <= 0 {
			return handler(ctx, req)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return handler(ctx, req)
	}
}

func unarySheddingInterceptor(shedder load.Shedder, metric func()) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if shedder == nil {
			return handler(ctx, req)
		}
		cb, err := shedder.Allow()
		if err != nil {
			if metric != nil {
				metric()
			}
			logx.Errorf("grpc shedding reject: %s", info.FullMethod)
			return nil, status.Error(codes.Unavailable, "server overloaded")
		}
		defer func() {
			if cb != nil {
				cb.Pass()
			}
		}()
		return handler(ctx, req)
	}
}

// --- Client Unary Interceptors ---

func clientTracingInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	propagator := otel.GetTextMapPropagator()
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.New(nil)
	}
	carrier := &metadataCarrier{md: md}
	propagator.Inject(ctx, carrier)
	ctx = metadata.NewOutgoingContext(ctx, carrier.md)

	tracer := otel.Tracer(trace.TraceName)
	spanCtx, span := tracer.Start(ctx, method,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(
			semconv.RPCServiceKey.String(method),
		),
	)
	defer span.End()

	err := invoker(spanCtx, method, req, reply, cc, opts...)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func clientBreakerInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	b := breaker.GetBreaker(method)
	return b.DoWithAcceptable(func() error {
		return invoker(ctx, method, req, reply, cc, opts...)
	}, func(err error) bool {
		return err == nil || status.Code(err) != codes.Internal
	})
}

func clientTimeoutInterceptor(timeout time.Duration) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if timeout <= 0 {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// --- Metadata Carrier for OTel ---

type metadataCarrier struct {
	md metadata.MD
}

func (c *metadataCarrier) Get(key string) string {
	if c.md == nil {
		return ""
	}
	vals := c.md.Get(key)
	if len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func (c *metadataCarrier) Set(key, value string) {
	if c.md == nil {
		c.md = metadata.New(nil)
	}
	c.md.Set(key, value)
}

func (c *metadataCarrier) Keys() []string {
	if c.md == nil {
		return nil
	}
	var keys []string
	for k := range c.md {
		keys = append(keys, k)
	}
	return keys
}
