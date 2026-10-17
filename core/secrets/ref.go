package secrets

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named secrets service instance.
// It carries no value: the container resolves it to exactly one Secrets
// built from Options.
func Ref(name string) ref.Ref[Secrets, Options] {
	return ref.Named[Secrets, Options]("secrets", name)
}

// DefaultRef returns the typed reference to the default secrets service
// instance.
func DefaultRef() ref.Ref[Secrets, Options] {
	return ref.Default[Secrets, Options]("secrets")
}
