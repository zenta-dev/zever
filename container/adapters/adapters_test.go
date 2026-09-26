package adapters

import (
	"testing"
)

// TestRegisterAll_idempotent pins the bundle contract the container relies
// on: registration only fills factory maps (no I/O, so safe at startup)
// and duplicate registrations are ignored, so RegisterAll and the
// per-family funcs can be mixed and repeated freely.
func TestRegisterAll_idempotent(t *testing.T) {
	t.Parallel()

	RegisterAll()
	RegisterAll()

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

	RegisterAll()
}
