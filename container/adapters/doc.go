// Package adapters registers heavyweight service adapters that the core
// container package deliberately no longer imports.
//
// container/services.go only wires zero- and light-infrastructure adapters,
// so that `go list -deps ./container` stays free of SDK-heavy families
// (LLM clients, cloud SDKs, billing SDKs, external search/vector clients,
// headless-Chrome rendering). Each family below exposes a Register function
// that fills the same process-wide service registries the container resolves
// through; registration opens nothing and starts no background work, so
// calling it is safe at startup and idempotent (duplicate registrations are
// ignored, matching the container's own wiring).
//
// There is no init wiring anywhere in this tree: the host application is the
// composition root and calls what it needs, typically once at startup:
//
//	import "github.com/zenta-dev/zever/container/adapters"
//
//	func main() {
//		adapters.RegisterAll()
//		// ... container.New(cfg) resolves heavy adapters from here on.
//	}
//
// Importing this package (even without calling anything) still pulls the
// heavy third-party SDKs into the build; splitting each family into its own
// Go module is the planned follow-up (W4 full split). Until then, prefer the
// per-family Register funcs over RegisterAll when binary weight matters.
package adapters
