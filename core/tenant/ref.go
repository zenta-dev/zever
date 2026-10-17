package tenant

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named tenant service instance. It
// carries no value: the container resolves it to exactly one Tenant built
// from Options.
func Ref(name string) ref.Ref[Tenant, Options] {
	return ref.Named[Tenant, Options]("tenant", name)
}

// DefaultRef returns the typed reference to the default tenant service
// instance.
func DefaultRef() ref.Ref[Tenant, Options] {
	return ref.Default[Tenant, Options]("tenant")
}
