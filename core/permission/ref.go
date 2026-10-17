package permission

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named permission service
// instance. It carries no value: the container resolves it to exactly one
// Checker built from Options.
func Ref(name string) ref.Ref[Checker, Options] {
	return ref.Named[Checker, Options]("permission", name)
}

// DefaultRef returns the typed reference to the default permission
// service instance.
func DefaultRef() ref.Ref[Checker, Options] {
	return ref.Default[Checker, Options]("permission")
}
