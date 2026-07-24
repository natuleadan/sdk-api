package runtime

import (
	"fmt"
	"strings"
	"sync"

	"github.com/natuleadan/sdk-api/infra/discov"
	"github.com/natuleadan/sdk-api/infra/logx"
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

// directResolverBuilder resolves direct:///host1,host2 to static addresses.
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

// etcdResolverBuilder resolves etcd:///host1,host2?key=service
// using discov.Subscriber to watch for endpoint changes.
type etcdResolverBuilder struct{}

func (*etcdResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	key := target.URL.Query().Get("key")
	hosts := strings.Split(target.Endpoint(), endpointSep)

	if key == "" || len(hosts) == 0 {
		return nil, fmt.Errorf("etcd resolver: key and hosts required")
	}

	sub, err := discov.NewSubscriber(hosts, key)
	if err != nil {
		return nil, fmt.Errorf("etcd resolver: new subscriber: %w", err)
	}

	r := &etcdResolver{
		sub:    sub,
		cc:     cc,
		hosts:  hosts,
		key:    key,
		closed: make(chan struct{}),
	}
	r.updateAddrs(sub.Values())

	sub.AddListener(func() {
		r.updateAddrs(sub.Values())
	})

	return r, nil
}

func (*etcdResolverBuilder) Scheme() string { return etcdScheme }

// etcdResolver watches etcd for endpoint changes via discov.Subscriber.
type etcdResolver struct {
	sub    *discov.Subscriber
	cc     resolver.ClientConn
	hosts  []string
	key    string
	closed chan struct{}
	mu     sync.Mutex
}

func (r *etcdResolver) updateAddrs(values []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	addrs := make([]resolver.Address, len(values))
	for i, v := range values {
		addrs[i] = resolver.Address{Addr: strings.TrimSpace(v)}
	}
	if len(addrs) == 0 {
		return
	}
	if err := r.cc.UpdateState(resolver.State{Addresses: addrs}); err != nil {
		logx.Errorf("etcd resolver: update state: %v", err)
	}
}

func (r *etcdResolver) ResolveNow(_ resolver.ResolveNowOptions) {}

func (r *etcdResolver) Close() {
	select {
	case <-r.closed:
		return
	default:
		close(r.closed)
	}
	r.sub.Close()
}

type nopResolver struct{}

func (*nopResolver) ResolveNow(resolver.ResolveNowOptions) {}
func (*nopResolver) Close()                                {}

func BuildDirectTarget(endpoints []string) string {
	return fmt.Sprintf("%s:///%s", directScheme, strings.Join(endpoints, endpointSep))
}

func BuildEtcdTarget(endpoints []string, key string) string {
	return fmt.Sprintf("%s:///%s?key=%s", etcdScheme, strings.Join(endpoints, endpointSep), key)
}
