package runtime

import (
	"fmt"
	"strings"

	"google.golang.org/grpc/resolver"
)

const (
	directScheme = "direct"
	etcdScheme   = "etcd"
	endpointSep  = ","
)

func init() {
	resolver.Register(&directResolverBuilder{})
	resolver.Register(&etcdResolverBuilder{})
}

type directResolverBuilder struct{}

func (*directResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	endpoints := strings.Split(target.Endpoint(), endpointSep)
	addrs := make([]resolver.Address, len(endpoints))
	for i, ep := range endpoints {
		addrs[i] = resolver.Address{Addr: strings.TrimSpace(ep)}
	}
	if err := cc.UpdateState(resolver.State{Addresses: addrs}); err != nil {
		return nil, fmt.Errorf("direct resolver: update state: %w", err)
	}
	return &nopResolver{}, nil
}

func (*directResolverBuilder) Scheme() string { return directScheme }

type etcdResolverBuilder struct{}

func (*etcdResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	key := target.URL.Query().Get("key")
	hosts := strings.Split(target.Endpoint(), endpointSep)

	_ = key
	_ = hosts

	addrs := []resolver.Address{}
	for _, h := range hosts {
		addrs = append(addrs, resolver.Address{Addr: strings.TrimSpace(h)})
	}
	if err := cc.UpdateState(resolver.State{Addresses: addrs}); err != nil {
		return nil, fmt.Errorf("etcd resolver: update state: %w", err)
	}
	return &nopResolver{}, nil
}

func (*etcdResolverBuilder) Scheme() string { return etcdScheme }

type nopResolver struct{}

func (*nopResolver) ResolveNow(resolver.ResolveNowOptions) {}
func (*nopResolver) Close()                                {}

func BuildDirectTarget(endpoints []string) string {
	return fmt.Sprintf("%s:///%s", directScheme, strings.Join(endpoints, endpointSep))
}

func BuildEtcdTarget(endpoints []string, key string) string {
	return fmt.Sprintf("%s:///%s?key=%s", etcdScheme, strings.Join(endpoints, endpointSep), key)
}
