package geo

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named geo service instance. It
// carries no value: the container resolves it to exactly one Geo built from
// Options.
func Ref(name string) ref.Ref[Geo, Options] {
	return ref.Named[Geo, Options]("geo", name)
}

// DefaultRef returns the typed reference to the default geo service
// instance.
func DefaultRef() ref.Ref[Geo, Options] {
	return ref.Default[Geo, Options]("geo")
}
