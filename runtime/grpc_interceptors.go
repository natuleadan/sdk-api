package runtime

import (
	"context"
	"strconv"
	"time"

	"github.com/natuleadan/sdk-api/infra/breaker"
	"github.com/natuleadan/sdk-api/infra/load"
	"github.com/natuleadan/sdk-api/infra/logx"
	"github.com/natuleadan/sdk-api/infra/metric"
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
	Trace      bool
	Breaker    bool
	Timeout    bool
	Shedding   bool
	Prometheus bool
}

func defaultGrpcInterceptorsConfig() GrpcInterceptorsConfig {
	return GrpcInterceptorsConfig{
		Trace:      true,
		Breaker:    true,
		Timeout:    true,
		Shedding:   true,
		Prometheus: true,
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
	spanCtx, span := tracer.Start(
		ctx, info.FullMethod,
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

func unaryRecoverInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
	defer func() {
		if r := recover(); r != nil {
			logx.Errorf("grpc panic recovered: %s: %v", info.FullMethod, r)
			err = status.Error(codes.Internal, "internal server error")
		}
	}()
	return handler(ctx, req)
}

// --- Stream Interceptors ---

func streamTracingInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	propagator := otel.GetTextMapPropagator()
	md, ok := metadata.FromIncomingContext(ss.Context())
	if ok {
		ctx := propagator.Extract(ss.Context(), &metadataCarrier{md: md})
		ss = &wrappedServerStream{ServerStream: ss, ctx: ctx}
	}

	tracer := otel.Tracer(trace.TraceName)
	_, span := tracer.Start(
		ss.Context(), info.FullMethod,
		oteltrace.WithSpanKind(oteltrace.SpanKindServer),
		oteltrace.WithAttributes(semconv.RPCServiceKey.String(info.FullMethod)),
	)
	defer span.End()

	err := handler(srv, ss)
	if err != nil {
		span.RecordError(err)
	}
	return err
}

func streamBreakerInterceptor(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	b := breaker.GetBreaker(info.FullMethod)
	return b.DoWithAcceptable(func() error {
		return handler(srv, ss)
	}, func(err error) bool {
		return err == nil || status.Code(err) != codes.Internal
	})
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
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
	spanCtx, span := tracer.Start(
		ctx, method,
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

// --- Prometheus Metrics ---

var (
	promServerDur = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "rpc_server",
		Subsystem: "requests",
		Name:      "duration_ms",
		Help:      "gRPC server request duration in milliseconds",
		Labels:    []string{"method"},
		Buckets:   []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000},
	})
	promServerCode = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "rpc_server",
		Subsystem: "requests",
		Name:      "code_total",
		Help:      "gRPC server request code count",
		Labels:    []string{"method", "code"},
	})
	promClientDur = metric.NewHistogramVec(&metric.HistogramVecOpts{
		Namespace: "rpc_client",
		Subsystem: "requests",
		Name:      "duration_ms",
		Help:      "gRPC client request duration in milliseconds",
		Labels:    []string{"method"},
		Buckets:   []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2000, 5000},
	})
	promClientCode = metric.NewCounterVec(&metric.CounterVecOpts{
		Namespace: "rpc_client",
		Subsystem: "requests",
		Name:      "code_total",
		Help:      "gRPC client request code count",
		Labels:    []string{"method", "code"},
	})
)

func unaryPrometheusInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	start := time.Now()
	resp, err := handler(ctx, req)
	promServerDur.Observe(time.Since(start).Milliseconds(), info.FullMethod)
	promServerCode.Inc(info.FullMethod, strconv.Itoa(int(status.Code(err))))
	return resp, err
}

func clientPrometheusInterceptor(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	start := time.Now()
	err := invoker(ctx, method, req, reply, cc, opts...)
	promClientDur.Observe(time.Since(start).Milliseconds(), method)
	promClientCode.Inc(method, strconv.Itoa(int(status.Code(err))))
	return err
}

// --- Metadata Carrier ---

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
