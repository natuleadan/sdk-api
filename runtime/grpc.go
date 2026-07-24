package runtime

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/natuleadan/sdk-api/infra/discov"
	"github.com/natuleadan/sdk-api/infra/load"
	"github.com/natuleadan/sdk-api/infra/logx"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

type GrpcRegisterFn func(server *grpc.Server)

type GrpcServer struct {
	server   *grpc.Server
	cfg      *GrpcServerConf
	listener net.Listener
	etcdPub  *discov.Publisher
}

func NewGrpcServer(cfg *GrpcServerConf, register GrpcRegisterFn, interceptorCfg ...GrpcInterceptorsConfig) (*GrpcServer, error) {
	if cfg == nil || cfg.ListenOn == "" {
		return nil, fmt.Errorf("grpc: listen_on is required")
	}

	lc := net.ListenConfig{}
	listener, err := lc.Listen(context.Background(), "tcp", cfg.ListenOn)
	if err != nil {
		return nil, fmt.Errorf("grpc: listen: %w", err)
	}

	ic := defaultGrpcInterceptorsConfig()
	if len(interceptorCfg) > 0 {
		ic = interceptorCfg[0]
	}

	opts := buildGrpcServerOptions(cfg, ic)
	opts = append(opts, grpc.KeepaliveParams(keepalive.ServerParameters{
		MaxConnectionIdle: 5 * time.Minute,
	}))

	server := grpc.NewServer(opts...)
	if register != nil {
		register(server)
	}

	if cfg.Health {
		healthServer := health.NewServer()
		grpc_health_v1.RegisterHealthServer(server, healthServer)
		healthServer.Resume()
	}

	reflection.Register(server)

	return &GrpcServer{
		server:   server,
		cfg:      cfg,
		listener: listener,
	}, nil
}

func buildGrpcServerOptions(cfg *GrpcServerConf, ic GrpcInterceptorsConfig) []grpc.ServerOption {
	var unary []grpc.UnaryServerInterceptor
	unary = append(unary, unaryRecoverInterceptor)
	if ic.Trace {
		unary = append(unary, unaryTracingInterceptor)
	}
	if ic.Breaker {
		unary = append(unary, unaryBreakerInterceptor)
	}
	if ic.Timeout && cfg.Timeout > 0 {
		unary = append(unary, unaryTimeoutInterceptor(time.Duration(cfg.Timeout)*time.Millisecond))
	}
	if ic.Shedding && cfg.CpuThreshold > 0 {
		shedder := load.NewAdaptiveShedder(load.WithCpuThreshold(cfg.CpuThreshold))
		unary = append(unary, unarySheddingInterceptor(shedder, nil))
	}
	if ic.Prometheus {
		unary = append(unary, unaryPrometheusInterceptor)
	}

	var stream []grpc.StreamServerInterceptor
	if ic.Trace {
		stream = append(stream, streamTracingInterceptor)
	}
	if ic.Breaker {
		stream = append(stream, streamBreakerInterceptor)
	}

	var opts []grpc.ServerOption
	if len(unary) > 0 {
		opts = append(opts, grpc.ChainUnaryInterceptor(unary...))
	}
	if len(stream) > 0 {
		opts = append(opts, grpc.ChainStreamInterceptor(stream...))
	}
	return opts
}

func (gs *GrpcServer) Start() {
	go func() {
		logx.Infof("gRPC server listening on %s", gs.cfg.ListenOn)
		if err := gs.server.Serve(gs.listener); err != nil {
			logx.Errorf("gRPC serve: %v", err)
		}
	}()
}

func (gs *GrpcServer) Stop() {
	if gs.server != nil {
		gs.server.GracefulStop()
	}
	if gs.etcdPub != nil {
		gs.etcdPub.Stop()
	}
}

func (gs *GrpcServer) Server() *grpc.Server {
	return gs.server
}

type GrpcClient struct {
	name string
	conn *grpc.ClientConn
	cfg  *GrpcClientConf
}

func NewGrpcClient(cfg *GrpcClientConf) (*GrpcClient, error) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("grpc: client name is required")
	}

	target := cfg.Target
	if len(cfg.Endpoints) > 0 {
		if len(cfg.Endpoints) == 1 {
			target = cfg.Endpoints[0]
		} else {
			target = BuildDirectTarget(cfg.Endpoints)
		}
	}
	if target == "" {
		return nil, fmt.Errorf("grpc: client %s: target or endpoints required", cfg.Name)
	}

	var dialOpts []grpc.DialOption
	if cfg.Secure {
		creds, err := credentials.NewClientTLSFromFile("", "")
		if err != nil {
			return nil, fmt.Errorf("grpc: client %s: tls: %w", cfg.Name, err)
		}
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	} else {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	ic := defaultGrpcInterceptorsConfig()
	var unaryInterceptors []grpc.UnaryClientInterceptor
	if ic.Trace {
		unaryInterceptors = append(unaryInterceptors, clientTracingInterceptor)
	}
	if ic.Breaker {
		unaryInterceptors = append(unaryInterceptors, clientBreakerInterceptor)
	}
	if ic.Timeout && cfg.Timeout > 0 {
		unaryInterceptors = append(unaryInterceptors, clientTimeoutInterceptor(time.Duration(cfg.Timeout)*time.Millisecond))
	}
	if ic.Prometheus {
		unaryInterceptors = append(unaryInterceptors, clientPrometheusInterceptor)
	}
	if len(unaryInterceptors) > 0 {
		dialOpts = append(dialOpts, grpc.WithChainUnaryInterceptor(unaryInterceptors...))
	}

	conn, err := grpc.NewClient(target, dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("grpc: dial %s: %w", cfg.Name, err)
	}
	conn.Connect()

	return &GrpcClient{
		name: cfg.Name,
		conn: conn,
		cfg:  cfg,
	}, nil
}

func (gc *GrpcClient) Conn() *grpc.ClientConn {
	return gc.conn
}

func (gc *GrpcClient) Name() string {
	return gc.name
}

func (gc *GrpcClient) Close() error {
	return gc.conn.Close()
}
