package password

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named password service instance.
// It carries no value: the container resolves it to exactly one Hasher
// built from Options.
func Ref(name string) ref.Ref[Hasher, Options] {
	return ref.Named[Hasher, Options]("password", name)
}

// DefaultRef returns the typed reference to the default password service
// instance.
func DefaultRef() ref.Ref[Hasher, Options] {
	return ref.Default[Hasher, Options]("password")
}
