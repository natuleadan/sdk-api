package runtime

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func findFreePort(t *testing.T) string {
	t.Helper()
	lc := net.ListenConfig{}
	lis, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := lis.Addr().String()
	lis.Close()
	return port
}

func TestNewGrpcServer_InvalidConfig(t *testing.T) {
	t.Parallel()

	_, err := NewGrpcServer(nil, nil)
	require.Error(t, err)

	_, err = NewGrpcServer(&GrpcServerConf{}, nil)
	require.Error(t, err)
}

func TestNewGrpcServer_StartAndStop(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gRPC server test in short mode")
	}
	t.Parallel()

	addr := findFreePort(t)
	cfg := &GrpcServerConf{
		ListenOn:     addr,
		Timeout:      5000,
		CpuThreshold: 0,
		Health:       true,
	}

	gs, err := NewGrpcServer(cfg, nil)
	require.NoError(t, err)
	require.NotNil(t, gs)

	gs.Start()
	defer gs.Stop()

	time.Sleep(100 * time.Millisecond)

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer conn.Close()

	healthClient := grpc_health_v1.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := healthClient.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
}

func TestNewGrpcClient_InvalidConfig(t *testing.T) {
	t.Parallel()

	_, err := NewGrpcClient(&GrpcClientConf{})
	require.Error(t, err)

	_, err = NewGrpcClient(&GrpcClientConf{Name: "test"})
	require.Error(t, err)
}

func TestNewGrpcServer_WithRegister(t *testing.T) {
	t.Parallel()

	addr := findFreePort(t)
	registered := false

	gs, err := NewGrpcServer(&GrpcServerConf{ListenOn: addr, Health: false}, func(s *grpc.Server) {
		registered = true
	})
	require.NoError(t, err)
	gs.Start()
	defer gs.Stop()

	assert.True(t, registered)
}

func TestNewGrpcClient_DirectEndpoint(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gRPC client test in short mode")
	}
	t.Parallel()

	addr := findFreePort(t)
	cfg := &GrpcServerConf{ListenOn: addr, Timeout: 5000}
	gs, err := NewGrpcServer(cfg, nil)
	require.NoError(t, err)
	gs.Start()
	defer gs.Stop()

	time.Sleep(100 * time.Millisecond)

	client, err := NewGrpcClient(&GrpcClientConf{
		Name:      "test-client",
		Endpoints: []string{addr},
		Timeout:   5000,
		NonBlock:  false,
	})
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Equal(t, "test-client", client.Name())
	assert.NotNil(t, client.Conn())
	client.Close()
}

func TestBuildDirectTarget(t *testing.T) {
	t.Parallel()

	target := BuildDirectTarget([]string{"localhost:8081", "localhost:8082"})
	assert.Contains(t, target, "direct:///")
	assert.Contains(t, target, "localhost:8081")
	assert.Contains(t, target, "localhost:8082")
}

func TestBuildEtcdTarget(t *testing.T) {
	t.Parallel()

	target := BuildEtcdTarget([]string{"127.0.0.1:2379"}, "my-service.rpc")
	assert.Contains(t, target, "etcd:///")
	assert.Contains(t, target, "127.0.0.1:2379")
	assert.Contains(t, target, "my-service.rpc")
}

func TestGrpcClient_Conn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping gRPC client test in short mode")
	}
	t.Parallel()

	addr := findFreePort(t)
	cfg := &GrpcServerConf{ListenOn: addr, Timeout: 5000, Health: true}
	gs, _ := NewGrpcServer(cfg, nil)
	gs.Start()
	defer gs.Stop()

	time.Sleep(100 * time.Millisecond)

	client, err := NewGrpcClient(&GrpcClientConf{
		Name:     "health-client",
		Target:   addr,
		Timeout:  5000,
		NonBlock: false,
	})
	require.NoError(t, err)
	defer client.Close()

	hc := grpc_health_v1.NewHealthClient(client.Conn())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := hc.Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	require.NoError(t, err)
	assert.Equal(t, grpc_health_v1.HealthCheckResponse_SERVING, resp.Status)
}
