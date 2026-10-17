package analytics

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named analytics service instance.
// It carries no value: the container resolves it to exactly one Analytics
// built from Options.
func Ref(name string) ref.Ref[Analytics, Options] {
	return ref.Named[Analytics, Options]("analytics", name)
}

// DefaultRef returns the typed reference to the default analytics service
// instance.
func DefaultRef() ref.Ref[Analytics, Options] {
	return ref.Default[Analytics, Options]("analytics")
}
