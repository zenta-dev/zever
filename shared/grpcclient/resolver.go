package grpcclient

import (
	"fmt"

	"google.golang.org/grpc/resolver"
)

// staticScheme is the URL scheme handled by staticResolverBuilder.
const staticScheme = "static"

// staticResolverBuilder is a resolver.Builder that resolves static:///name
// targets from a fixed address map. It is registered per-connection via
// grpc.WithResolvers, never through the global resolver registry.
type staticResolverBuilder struct {
	addrs map[string][]string
}

// Scheme returns the scheme handled by the builder: "static".
func (b *staticResolverBuilder) Scheme() string { return staticScheme }

// Build resolves the target name against the address map and reports the
// resulting addresses to cc. An unknown or empty name is an error, so the
// channel fails fast instead of dialing nothing.
func (b *staticResolverBuilder) Build(target resolver.Target, cc resolver.ClientConn, _ resolver.BuildOptions) (resolver.Resolver, error) {
	name := target.Endpoint()
	if name == "" {
		name = target.URL.Host
	}

	addrs, ok := b.addrs[name]
	if !ok || len(addrs) == 0 {
		return nil, fmt.Errorf("grpcclient: static resolver: unknown name %q", name)
	}

	addresses := make([]resolver.Address, 0, len(addrs))
	for _, a := range addrs {
		addresses = append(addresses, resolver.Address{Addr: a})
	}

	if err := cc.UpdateState(resolver.State{Addresses: addresses}); err != nil {
		return nil, fmt.Errorf("grpcclient: static resolver: update state: %w", err)
	}

	return &staticResolver{}, nil
}

// staticResolver is a no-op resolver: the address set is fixed at build
// time and never re-resolved.
type staticResolver struct{}

// ResolveNow is a no-op: the address set never changes.
func (r *staticResolver) ResolveNow(resolver.ResolveNowOptions) {}

// Close is a no-op: the builder holds no resources.
func (r *staticResolver) Close() {}
