package i18n

import "github.com/zenta-dev/zever/shared/ref"

// Ref returns the typed reference to the named i18n service instance. It
// carries no value: the container resolves it to exactly one I18n built
// from Options.
func Ref(name string) ref.Ref[I18n, Options] {
	return ref.Named[I18n, Options]("i18n", name)
}

// DefaultRef returns the typed reference to the default i18n service
// instance.
func DefaultRef() ref.Ref[I18n, Options] {
	return ref.Default[I18n, Options]("i18n")
}
