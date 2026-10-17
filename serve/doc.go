// Package serve hosts the shared boot/mount/shutdown runtime every
// zever-generated server binary runs: resolve config, build the container,
// mount modules on the resolved router, serve HTTP (and opt-in gRPC),
// answer health probes, and shut down gracefully on SIGINT/SIGTERM.
//
// Serve is adapter-agnostic: it imports core batteries and shared leaves
// only, never an adapters/* module. Adapter registration stays in the
// generated app package (explicit imports plus Register calls), which
// passes providers into Main and Worker.
//
// Environment (explicit Option wins over process env over default):
//
//	PORT            HTTP listen port, default 8080 (WithPort)
//	GRPC_ADDR       gRPC listen address, empty disables gRPC (WithGRPCAddr)
//	PPROF_ADDR      loopback pprof address, empty disables it (WithPprofAddr)
//	GRPC_REFLECTION true/false gating gRPC reflection; unset follows ZEVER_ENV
//	ZEVER_ENV       dev/test/development marks non-production (reflection,
//	                deterministic dev crypto key)
//
// Generated main.go contract (stream D): call Main with the app's providers
// and a modules func returning one Module per schema module. Each Module's
// Mount calls that module's generated RegisterModule, resolving the gRPC
// server via c.GRPC, auth via c.Auth, and permission via c.Permission. On
// Main error, print to stderr and os.Exit(1). The `doctor` (or `--doctor`)
// argument prints the resolve-all JSON report to stdout instead of serving;
// see doctor.go for the exact document shape the CLI parses.
//
// Module shape choice: the generated RegisterModule wrapper needs the HTTP
// router, the shared gRPC server, auth, permission, and ModuleImpls. Mount
// takes only (r, c): the container already owns the gRPC singleton
// (c.GRPC, built by serve with the fail-closed interceptor chain before
// Mount runs), so modules resolve everything else from c without extra
// parameters. Per-method gRPC policies travel via the optional GRPCModule:
// serve merges them into the single interceptor before construction.
// A module needing gRPC registers its services inside Mount and declares
// its policies via GRPCPolicies; without a GRPC_ADDR the boot fails closed.
package serve
