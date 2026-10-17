package db

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named db service instance. It
// carries no value: the container resolves it to exactly one DB built from
// Options.
func Ref(name string) ref.Ref[DB, Options] {
	return ref.Named[DB, Options]("db", name)
}

// DefaultRef returns the typed reference to the default db service
// instance.
func DefaultRef() ref.Ref[DB, Options] {
	return ref.Default[DB, Options]("db")
}
