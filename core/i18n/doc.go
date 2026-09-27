// Package i18n looks up locale messages with swappable adapters.
//
// It resolves Translate and Locales over catalogs with fallback chains and template interpolation. It is not a translation service or CMS; catalogs come from embedded files or a remote endpoint.
//
// Type safety: I18n plus typed Options plus Adapter enum plus Factory. EmbedOptions and RemoteOptions carry per-backend settings; BaseLocale and LocaleChain compute typed lookup chains. Unsupported features fail closed.
//
// DX: Open with Open, custom backends with Register. Options are typed with zero-infra defaults for tests. Config file plus env I18N_<FIELD> (no prefix, e.g. I18N_ADAPTER). See config/README.md.
//
// Container: container.New(cfg) then c.I18n(). Lazy per-service singleton, retry on error. See container/README.md.
//
// Lifecycle: ctx is first arg for IO, never stored. Close releases resources; only resolved services close. I18n releases its backend with Close() which takes no ctx.
//
// Errors: sentinel errors, errors.Is compatible, prefixed i18n:. Name service and field only in errors.
//
// Security: never log secrets or raw option maps, use config.RedactedServices. APIKey is never logged, https is required unless AllowInsecure is set, and locale file names are validated.
//
// Performance: catalogs parse once into compiled templates; remote calls use the default 30s timeout with MaxInFlight capping concurrency. Bounded pools and timeouts.
//
// Concurrency: safe for concurrent use unless noted. No globals, no init wiring.
//
// Example: see ExampleOpen in example_test.go.
package i18n
