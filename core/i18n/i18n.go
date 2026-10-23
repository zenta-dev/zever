package i18n

import (
	"context"
)

// I18n defines the core operations for internationalized message lookup.
type I18n interface {
	// Translate returns the message for locale and key, interpolating args.
	Translate(ctx context.Context, locale, key string, args map[string]string) (string, error)
	// Locales lists the available locales.
	Locales(ctx context.Context) ([]string, error)
	// Close shuts down the backend and releases associated resources.
	Close() error
}

// Factory creates an I18n from the given Options.
type Factory func(opts Options) (I18n, error)
