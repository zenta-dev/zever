package main

import (
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- shared scaffolding test helpers (server, worker and seed) ---
//
// validateGoSyntax and readGenerated are owned by the generate wave; other
// command-test waves reuse them without redefining. writeGoMod is
// generate-local.

// writeGoMod writes a minimal go.mod into dir, the precondition every
// entrypoint scaffold has: the generated file imports the target project's own
// internal/app package, so it must know that project's module path.
func writeGoMod(t *testing.T, dir, modulePath string) {
	t.Helper()

	content := "module " + modulePath + "\n\ngo 1.24\n"

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
}

// validateGoSyntax proves content parses as syntactically valid Go and is
// already gofmt-formatted, mirroring the zenorm backend's own generated
// -code check. It only checks syntax; it never resolves imported packages.
func validateGoSyntax(t *testing.T, filename string, content []byte) {
	t.Helper()

	fset := token.NewFileSet()

	if _, err := parser.ParseFile(fset, filename, content, parser.AllErrors); err != nil {
		t.Fatalf("go/parser failed to parse generated file %s: %v\n--- content ---\n%s", filename, err, content)
	}

	formatted, err := format.Source(content)
	if err != nil {
		t.Fatalf("go/format.Source failed on generated file %s: %v", filename, err)
	}

	if string(formatted) != string(content) {
		t.Fatalf("generated file %s is not gofmt-formatted:\n--- got ---\n%s\n--- formatted ---\n%s",
			filename, content, formatted)
	}
}

// readGenerated reads a scaffolded file and asserts it is valid, gofmt-clean Go.
func readGenerated(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("expected a scaffold at %q: %v", path, err)
	}

	validateGoSyntax(t, path, data)

	return string(data)
}

// --- generate server ---

func TestRunGenerateServer(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("runGenerateServer: %v", err)
	}

	main := readGenerated(t, filepath.Join(dir, "cmd", "server", "main.go"))

	for _, fragment := range []string{
		`"example.com/shop/internal/app"`,
		`"github.com/zenta-dev/zever/middleware"`,
		"func main()",
		"httpServer.Shutdown(shutdownCtx)",
		"grpcServer.GracefulStop()",
		"c.Router()",
		"middleware.RecoverUnaryServerInterceptor(logger)",
		"middleware.TracingUnaryServerInterceptor(obs)",
		"authz.UnaryServerInterceptor(authInst, permInst, policies)",
		"grpc.KeepaliveParams(keepalive.ServerParameters{",
		"grpc.MaxRecvMsgSize(grpcMaxMessageBytes)",
		"grpc.MaxSendMsgSize(grpcMaxMessageBytes)",
		"r.Use(middleware.Recover(logger), middleware.RequestLogger(logger), middleware.Tracing(obs))",
		`r.Handle("GET", "/healthz",`,
		`r.Handle("GET", "/readyz",`,
		"database.Ping(pingCtx)",
		"ReadTimeout:       readTimeout,",
		"WriteTimeout:      writeTimeout,",
		"IdleTimeout:       idleTimeout,",
		`grpcAddr := flag.String("grpc-addr", ":9090", "gRPC address to listen on")`,
	} {
		if !strings.Contains(main, fragment) {
			t.Fatalf("server main.go lacks %q:\n%s", fragment, main)
		}
	}

	// zever's zero-infra defaults ship a real rate limit (rate 10, burst 20),
	// so rate-limit wiring is on even with no project config -- the inverse
	// of zen-go's opt-in. See rateLimitConfigured.
	for _, wanted := range []string{"c.Ratelimit()", "middleware.RateLimit", "RateLimitUnaryServerInterceptor"} {
		if !strings.Contains(main, wanted) {
			t.Fatalf("server main.go must wire rate limiting under zever defaults, lacks %q:\n%s", wanted, main)
		}
	}

	// The app package is scaffolded alongside it when the project has none.
	app := readGenerated(t, filepath.Join(dir, "internal", "app", "app.go"))

	for _, fragment := range []string{
		"package app",
		`_ "github.com/zenta-dev/zever/router/stdhttp"`,
		// The zero-infra defaults every generated server resolves
		// unconditionally must have their adapters registered here: this
		// file is scaffolded once and never regenerated, so a battery added
		// to the server template later can't rely on a later re-scaffold to
		// pick up its adapter import.
		`_ "github.com/zenta-dev/zever/auth/jwt"`,
		`_ "github.com/zenta-dev/zever/permission/noop"`,
		`_ "github.com/zenta-dev/zever/observability/stdout"`,
		`_ "github.com/zenta-dev/zever/ratelimit/memory"`,
		`_ "github.com/zenta-dev/zever/log/slog"`,
		`_ "github.com/zenta-dev/zever/db/sqlite"`,
		`_ "github.com/zenta-dev/zever/queue/memory"`,
		`_ "github.com/zenta-dev/zever/scheduler/embedded"`,
		"func New() (*container.Container, error)",
	} {
		if !strings.Contains(app, fragment) {
			t.Fatalf("app.go lacks %q:\n%s", fragment, app)
		}
	}

	assertGolden(t, "generate_server_main_default.golden", []byte(main))
	assertGolden(t, "generate_server_app.golden", []byte(app))
}

func TestRunGenerateServerRefusesToClobberWithoutForce(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	path := filepath.Join(dir, "cmd", "server", "main.go")

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	const handwritten = "package main\n\nfunc main() {}\n"

	if err := os.WriteFile(path, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runGenerateServer(nil)
	if err == nil {
		t.Fatalf("expected an error for an existing entrypoint")
	}

	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error does not mention --force: %v", err)
	}

	if got := readFile(t, path); got != handwritten {
		t.Fatalf("hand-written file was clobbered: %q", got)
	}

	// --force overwrites it.
	if err := runGenerateServer([]string{"--force"}); err != nil {
		t.Fatalf("runGenerateServer --force: %v", err)
	}

	if got := readFile(t, path); got == handwritten {
		t.Fatalf("--force did not overwrite the entrypoint")
	}
}

// TestRunGenerateServerKeepsAnExistingAppPackage proves ensureAppPackage is a
// create-if-missing, never an overwrite: internal/app is where a project keeps
// its hand-picked adapter imports.
func TestRunGenerateServerKeepsAnExistingAppPackage(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	appPath := filepath.Join(dir, "internal", "app", "app.go")

	if err := os.MkdirAll(filepath.Dir(appPath), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	const handwritten = "package app\n\n// hand written\n"

	if err := os.WriteFile(appPath, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("runGenerateServer: %v", err)
	}

	if got := readFile(t, appPath); got != handwritten {
		t.Fatalf("existing app.go was overwritten: %q", got)
	}
}

func TestRunGenerateServerHonoursProjectConfig(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), []byte("project:\n  server_entry: cmd/api\n"), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("runGenerateServer: %v", err)
	}

	readGenerated(t, filepath.Join(dir, "cmd", "api", "main.go"))

	if _, err := os.Stat(filepath.Join(dir, "cmd", "server", "main.go")); !os.IsNotExist(err) {
		t.Fatalf("scaffold ignored project.server_entry")
	}
}

func TestRunGenerateServerWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateServer(nil); err == nil {
		t.Fatalf("expected an error outside a Go module")
	}
}

func TestGoModulePath(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		wantErr bool
	}{
		{"plain", "module example.com/shop\n\ngo 1.24\n", "example.com/shop", false},
		{"quoted", "module \"example.com/shop\"\n", "example.com/shop", false},
		{"indented after a comment", "// c\n  module example.com/shop\n", "example.com/shop", false},
		{"bare module keyword", "module\n", "", true},
		{"no module line", "go 1.24\n", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDir(t, dir)

			if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(tc.content), 0o600); err != nil {
				t.Fatalf("write go.mod: %v", err)
			}

			got, err := goModulePath()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %q", tc.content)
				}

				return
			}

			if err != nil {
				t.Fatalf("goModulePath: %v", err)
			}

			if got != tc.want {
				t.Fatalf("goModulePath = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRenderGoFileRejectsInvalidGo(t *testing.T) {
	if _, err := renderGoFile("test", "bad.go", "package main\nfunc {", nil); err == nil {
		t.Fatalf("expected an error for a template that renders invalid Go")
	}

	if _, err := renderGoFile("test", "bad.go", "{{.Missing", nil); err == nil {
		t.Fatalf("expected an error for an unparseable template")
	}
}

// --- schema-aware wiring ---

const serverSchemaFixture = `entity Task {
	id: uuid @primary
	title: string
}

service TaskService {
	rpc GetTask(id: uuid) -> Task {
		http: GET "/tasks/{id}"
		auth: required
	}
}`

// writeServerSchema writes serverSchemaFixture under schema/app.zen relative
// to the current working directory, mirroring how a project's own
// auto-discovered schema dir looks.
func writeServerSchema(t *testing.T, dir string) {
	t.Helper()

	schemaDir := filepath.Join(dir, "schema")
	if err := os.MkdirAll(schemaDir, 0o750); err != nil {
		t.Fatalf("mkdir schema: %v", err)
	}

	if err := os.WriteFile(filepath.Join(schemaDir, "app.zen"), []byte(serverSchemaFixture), 0o600); err != nil {
		t.Fatalf("write schema/app.zen: %v", err)
	}
}

// TestRunGenerateServerWiresRegisterModule proves that, given a schema (auto-
// discovered under schema/), the scaffolded main.go actually imports the
// generated gogen package, merges its GRPCPolicies(), constructs a stub
// service implementation, and calls RegisterModule -- no TODOs left for a
// declared module.
func TestRunGenerateServerWiresRegisterModule(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")
	writeServerSchema(t, dir)

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("runGenerateServer: %v", err)
	}

	main := readGenerated(t, filepath.Join(dir, "cmd", "server", "main.go"))

	for _, fragment := range []string{
		`genapp "example.com/shop/generated/gogen/app"`,
		`appimpl "example.com/shop/internal/service/app"`,
		"for k, v := range genapp.GRPCPolicies()",
		"taskServiceImpl := appimpl.NewTaskServiceImpl()",
		"genapp.RegisterModule(r, grpcServer, authInst, permInst, genapp.ModuleImpls{",
		"TaskService: taskServiceImpl,",
	} {
		if !strings.Contains(main, fragment) {
			t.Fatalf("wired server main.go lacks %q:\n%s", fragment, main)
		}
	}

	if strings.Contains(main, "TODO: pass .zen schema files") {
		t.Fatalf("wired server main.go should not show the no-schema TODO hint:\n%s", main)
	}

	stub := readGenerated(t, filepath.Join(dir, "internal", "service", "app", "task_service.go"))

	for _, fragment := range []string{
		"package appimpl",
		"type TaskServiceImpl struct{}",
		"func NewTaskServiceImpl() *TaskServiceImpl",
		"apperror.New(apperror.Unimplemented, \"GetTask not implemented\")",
	} {
		if !strings.Contains(stub, fragment) {
			t.Fatalf("service stub lacks %q:\n%s", fragment, stub)
		}
	}

	assertGolden(t, "generate_server_main_wired.golden", []byte(main))
}

// TestRunGenerateServerNeverOverwritesAnExistingStub proves the skip-if-
// exists contract: re-running after a developer has started implementing a
// stub must leave it untouched.
func TestRunGenerateServerNeverOverwritesAnExistingStub(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")
	writeServerSchema(t, dir)

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("runGenerateServer: %v", err)
	}

	stubPath := filepath.Join(dir, "internal", "service", "app", "task_service.go")

	const handwritten = "package appimpl\n\n// hand written business logic\n"

	if err := os.WriteFile(stubPath, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write handwritten stub: %v", err)
	}

	if err := runGenerateServer([]string{"--force"}); err != nil {
		t.Fatalf("runGenerateServer --force: %v", err)
	}

	if got := readFile(t, stubPath); got != handwritten {
		t.Fatalf("re-running generate server overwrote a hand-implemented stub: %q", got)
	}
}

// TestRunGenerateServerWiresRateLimitWhenConfigured proves rate-limit wiring
// appears when zever.yaml configures a rate/burst for the "ratelimit"
// battery. Under zever's defaults the wiring is already on (see
// TestRunGenerateServer), so this test pins the explicit-config path too.
func TestRunGenerateServerWiresRateLimitWhenConfigured(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	const cfg = "ratelimit:\n  adapter: memory\n  options:\n    rate: 100\n    burst: 20\n"

	if err := os.WriteFile(filepath.Join(dir, "zever.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write zever.yaml: %v", err)
	}

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("runGenerateServer: %v", err)
	}

	main := readGenerated(t, filepath.Join(dir, "cmd", "server", "main.go"))

	for _, fragment := range []string{
		"limiter, err := c.Ratelimit()",
		"middleware.RateLimitUnaryServerInterceptor(limiter, middleware.PeerAddrKey)",
		"r.Use(middleware.RateLimit(limiter, middleware.RemoteAddrKey))",
	} {
		if !strings.Contains(main, fragment) {
			t.Fatalf("server main.go lacks %q with a configured rate limit:\n%s", fragment, main)
		}
	}
}

// TestCoreBatterySelectionsMatchDefaults pins the adapter floor every
// generated entrypoint needs: one selection per core battery, each matching
// config.Default()'s own adapter pick, so app.go's blank imports can never
// drift from what the entrypoints resolve.
func TestCoreBatterySelectionsMatchDefaults(t *testing.T) {
	sel := coreBatterySelections()

	if len(sel) != len(coreBatteries) {
		t.Fatalf("coreBatterySelections = %d entries, want %d", len(sel), len(coreBatteries))
	}

	for _, fragment := range []string{
		"github.com/zenta-dev/zever/auth/jwt",
		"github.com/zenta-dev/zever/db/sqlite",
		"github.com/zenta-dev/zever/router/stdhttp",
	} {
		found := false

		for _, s := range sel {
			if batteryImportPath(s) == fragment {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("core selections lack %q: %v", fragment, sel)
		}
	}
}
