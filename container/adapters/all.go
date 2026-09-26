package adapters

// RegisterAll registers every heavyweight adapter family the core container
// no longer wires itself. It performs no I/O and is idempotent: duplicate
// registrations are ignored, so calling it more than once (or mixing it
// with per-family Register calls) is safe. Prefer the per-family Register
// funcs when only a subset is needed.
func RegisterAll() {
	RegisterAI()
	RegisterCloud()
	RegisterPayments()
	RegisterSearchVector()
	RegisterDoc()
	RegisterNotify()
	RegisterWeb()
	RegisterPermission()
	RegisterAnalytics()
	RegisterGeo()
}
