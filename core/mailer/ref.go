package mailer

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named mailer service instance. It
// carries no value: the container resolves it to exactly one Mailer built
// from Options.
func Ref(name string) ref.Ref[Mailer, Options] {
	return ref.Named[Mailer, Options]("mailer", name)
}

// DefaultRef returns the typed reference to the default mailer service
// instance.
func DefaultRef() ref.Ref[Mailer, Options] {
	return ref.Default[Mailer, Options]("mailer")
}
