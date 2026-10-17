package auth

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named auth service instance. It
// carries no value: the container resolves it to exactly one Auth built
// from Options.
func Ref(name string) ref.Ref[Auth, Options] {
	return ref.Named[Auth, Options]("auth", name)
}

// DefaultRef returns the typed reference to the default auth service
// instance.
func DefaultRef() ref.Ref[Auth, Options] {
	return ref.Default[Auth, Options]("auth")
}
