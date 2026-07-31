package runtime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

func TestRegisterGrpcService_Wiring(t *testing.T) {
	svc, err := newFromConfig(&ServiceConfig{
		Name: "test",
		Entry: []EntryDef{
			{Type: "grpc", ServiceName: "Greeter"},
		},
	})
	require.NoError(t, err)

	called := false
	var captured *grpc.Server
	svc.RegisterGrpcService("Greeter", func(srv *grpc.Server) {
		called = true
		captured = srv
	})

	addr := findFreePort(t)
	gs, err := NewGrpcServer(&GrpcServerConf{ListenOn: addr, Health: false}, nil)
	require.NoError(t, err)
	svc.grpcServer = gs

	svc.registerGrpcEntries()

	assert.True(t, called, "registered service should be invoked")
	assert.NotNil(t, captured, "service should receive the grpc server")
}

func TestRegisterGrpcService_NoMatch(t *testing.T) {
	svc, err := newFromConfig(&ServiceConfig{
		Name: "test",
		Entry: []EntryDef{
			{Type: "grpc", ServiceName: "OtherService"},
		},
	})
	require.NoError(t, err)

	called := false
	svc.RegisterGrpcService("Greeter", func(_ *grpc.Server) { called = true })

	addr := findFreePort(t)
	gs, err := NewGrpcServer(&GrpcServerConf{ListenOn: addr, Health: false}, nil)
	require.NoError(t, err)
	svc.grpcServer = gs

	svc.registerGrpcEntries()

	assert.False(t, called, "non-matching service should not be invoked")
}

func TestRegisterGrpcService_NoServer(t *testing.T) {
	svc, err := newFromConfig(&ServiceConfig{
		Name:  "test",
		Entry: []EntryDef{{Type: "grpc", ServiceName: "Greeter"}},
	})
	require.NoError(t, err)

	called := false
	svc.RegisterGrpcService("Greeter", func(_ *grpc.Server) { called = true })

	svc.registerGrpcEntries()

	assert.False(t, called, "no grpc server means no invocation")
}

func TestGrpcCall_NilClient(t *testing.T) {
	_, err := GrpcCall(context.Background(), nil, func(conn ClientConnInterface) (string, error) {
		return "called", nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "client not available")
}

func TestGrpcCall_ExecutesCallback(t *testing.T) {
	addr := findFreePort(t)
	gs, err := NewGrpcServer(&GrpcServerConf{ListenOn: addr, Health: true, Timeout: 5000}, nil)
	require.NoError(t, err)
	gs.Start()
	defer gs.Stop()

	client, err := NewGrpcClient(&GrpcClientConf{
		Name:     "test",
		Target:   addr,
		Timeout:  5000,
		NonBlock: false,
	})
	require.NoError(t, err)
	defer client.Close()

	executed := false
	val, err := GrpcCall(context.Background(), client, func(_ ClientConnInterface) (string, error) {
		executed = true
		return "ok", nil
	})
	require.NoError(t, err)
	assert.True(t, executed)
	assert.Equal(t, "ok", val)
}

func TestMustGetGrpcClientConn_NotConfigured(t *testing.T) {
	svc, err := newFromConfig(&ServiceConfig{Name: "test"})
	require.NoError(t, err)

	conn := MustGetGrpcClientConn(svc, "missing")
	assert.Nil(t, conn)
}

func TestMustGetGrpcServer_WithServer(t *testing.T) {
	svc, err := newFromConfig(&ServiceConfig{Name: "test"})
	require.NoError(t, err)

	addr := findFreePort(t)
	gs, err := NewGrpcServer(&GrpcServerConf{ListenOn: addr, Health: false}, nil)
	require.NoError(t, err)
	svc.grpcServer = gs

	srv := MustGetGrpcServer(svc)
	assert.NotNil(t, srv)
}

func TestGrpcCall_TypeAliasCompatibility(t *testing.T) {
	// ClientConnInterface must be assignable to the grpc interface
	var _ ClientConnInterface = (*grpc.ClientConn)(nil)
}
