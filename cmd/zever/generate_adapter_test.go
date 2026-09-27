package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// generateAdapter runs the command in a fresh temp working directory and
// returns that directory.
func generateAdapter(t *testing.T, args ...string) string {
	t.Helper()

	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateAdapter(args); err != nil {
		t.Fatalf("runGenerateAdapter(%q): %v", args, err)
	}

	return dir
}

func TestRunGenerateAdapterCache(t *testing.T) {
	dir := generateAdapter(t, "cache", "memcached")

	adapter := readGenerated(t, filepath.Join(dir, "adapters", "cache", "memcached", "memcached.go"))

	for _, fragment := range []string{
		"package memcached",
		`"github.com/zenta-dev/zever/core/cache"`,
		"func New(_ cache.Options) (cache.Cache, error)",
		"core/cache/adapter.go",
		"scaffolded register.go",
		"type adapter struct{}",
		`errors.New("[cache] memcached: not implemented")`,
		// every method of cache.Cache, with its real signature
		"func (a *adapter) Get(ctx context.Context, key string) ([]byte, error)",
		"func (a *adapter) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error",
		"func (a *adapter) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)",
		"func (a *adapter) Delete(ctx context.Context, key string) error",
		"func (a *adapter) Increment(ctx context.Context, key string) error",
		"func (a *adapter) Decrement(ctx context.Context, key string) error",
		"func (a *adapter) Exists(ctx context.Context, key string) (bool, error)",
		"func (a *adapter) Close(ctx context.Context) error",
		"// TODO: implement Get.",
		"return zero, errNotImplemented",
	} {
		if !strings.Contains(adapter, fragment) {
			t.Fatalf("generated adapter lacks %q:\n%s", fragment, adapter)
		}
	}

	// The scaffold must say out loud that it is not an implementation.
	for _, fragment := range []string{"SCAFFOLD", "TODO: implement"} {
		if !strings.Contains(adapter, fragment) {
			t.Fatalf("generated adapter lacks the %q disclaimer:\n%s", fragment, adapter)
		}
	}

	options := readGenerated(t, filepath.Join(dir, "adapters", "cache", "memcached", "options.go"))

	for _, fragment := range []string{
		"package memcached",
		"type Options struct{}",
		"func ParseOptions(_ map[string]any) (Options, error)",
	} {
		if !strings.Contains(options, fragment) {
			t.Fatalf("generated options.go lacks %q:\n%s", fragment, options)
		}
	}

	register := readGenerated(t, filepath.Join(dir, "adapters", "cache", "memcached", "register.go"))

	for _, fragment := range []string{
		"package memcached",
		`"github.com/zenta-dev/zever/core/cache"`,
		"func Register()",
		`cache.ParseAdapter("memcached")`,
		"cache.Register(adapter, New)",
	} {
		if !strings.Contains(register, fragment) {
			t.Fatalf("generated register.go lacks %q:\n%s", fragment, register)
		}
	}

	assertGolden(t, "generate_adapter_cache.golden", []byte(adapter))
	assertGolden(t, "generate_adapter_register.golden", []byte(register))
}

// TestRunGenerateAdapterInterfaceNameDiffersFromPackage covers the batteries
// whose interface is not named after their package — the case that makes the
// lookup table necessary in the first place.
func TestRunGenerateAdapterInterfaceNameDiffersFromPackage(t *testing.T) {
	cases := map[string]string{
		"ratelimit":    "ratelimit.Limiter",
		"permission":   "permission.Checker",
		"notification": "notification.Notifier",
		"idempotency":  "idempotency.Store",
		"session":      "session.Store",
	}

	for battery, want := range cases {
		t.Run(battery, func(t *testing.T) {
			dir := generateAdapter(t, battery, "acme")

			src := readGenerated(t, filepath.Join(dir, "adapters", battery, "acme", "acme.go"))

			if !strings.Contains(src, "func New(_ "+battery+".Options) ("+want+", error)") {
				t.Fatalf("want New returning %s:\n%s", want, src)
			}
		})
	}
}

// TestRunGenerateAdapterRouterEmbeddedHandler proves the table carries the
// methods router.Router inherits from its embedded http.Handler, which are
// invisible in the interface literal.
func TestRunGenerateAdapterRouterEmbeddedHandler(t *testing.T) {
	dir := generateAdapter(t, "router", "mux2")

	src := readGenerated(t, filepath.Join(dir, "adapters", "router", "mux2", "mux2.go"))

	if !strings.Contains(src, "func (a *adapter) ServeHTTP(w http.ResponseWriter, r *http.Request)") {
		t.Fatalf("router stub lacks ServeHTTP from the embedded http.Handler:\n%s", src)
	}
}

func TestRunGenerateAdapterFields(t *testing.T) {
	dir := generateAdapter(t,
		"storage", "acme",
		"--field", "api_key:string",
		"--field", "timeout:duration",
		"--field", "max_retries:int",
		"--field", "endpoints:strings",
		"--field", "use_tls:bool",
		"--field", "extra:map",
	)

	options := readGenerated(t, filepath.Join(dir, "adapters", "storage", "acme", "options.go"))

	for _, fragment := range []string{
		`"time"`,
		"APIKey string `json:\"api_key\" toml:\"api_key\" yaml:\"api_key\"`",
		"Timeout time.Duration `json:\"timeout\" toml:\"timeout\" yaml:\"timeout\"`",
		"MaxRetries int `json:\"max_retries\" toml:\"max_retries\" yaml:\"max_retries\"`",
		"Endpoints []string `json:\"endpoints\" toml:\"endpoints\" yaml:\"endpoints\"`",
		"UseTLS bool `json:\"use_tls\" toml:\"use_tls\" yaml:\"use_tls\"`",
		"Extra map[string]any `json:\"extra\" toml:\"extra\" yaml:\"extra\"`",
		"func ParseOptions(m map[string]any) (Options, error)",
		`o.APIKey = fieldOr[string](m, "api_key", "")`,
		`o.Timeout = fieldOr[time.Duration](m, "timeout", 0)`,
		`o.MaxRetries = fieldOr[int](m, "max_retries", 0)`,
		`o.Endpoints = fieldOr[[]string](m, "endpoints", nil)`,
		`o.UseTLS = fieldOr[bool](m, "use_tls", false)`,
		`o.Extra = fieldOr[map[string]any](m, "extra", nil)`,
		"func fieldOr[T any](m map[string]any, key string, def T) T",
	} {
		// gofmt aligns struct tags, so compare on whitespace-collapsed text.
		if !strings.Contains(collapseSpaces(options), collapseSpaces(fragment)) {
			t.Fatalf("generated options.go lacks %q:\n%s", fragment, options)
		}
	}

	assertGolden(t, "generate_adapter_options_fields.golden", []byte(options))
}

func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func TestRunGenerateAdapterFieldsBeforePositionals(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateAdapter([]string{"--field", "api_key:string", "cache", "acme"}); err != nil {
		t.Fatalf("runGenerateAdapter: %v", err)
	}

	options := readGenerated(t, filepath.Join(dir, "adapters", "cache", "acme", "options.go"))
	if !strings.Contains(options, "APIKey") {
		t.Fatalf("flags before positionals did not take effect:\n%s", options)
	}
}

func TestRunGenerateAdapterUnknownBattery(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	err := runGenerateAdapter([]string{"kafkaesque", "acme"})
	if err == nil {
		t.Fatal("want error for unknown battery")
	}

	msg := err.Error()

	if !strings.Contains(msg, `unknown battery "kafkaesque"`) {
		t.Fatalf("error should name the bad battery: %v", err)
	}

	// The error must list the valid names, in sorted order.
	for _, name := range []string{"cache", "ratelimit", "tenant", "workflow"} {
		if !strings.Contains(msg, name) {
			t.Fatalf("error should list %q as a valid battery: %v", name, err)
		}
	}

	if _, statErr := os.Stat(filepath.Join(dir, "kafkaesque")); !os.IsNotExist(statErr) {
		t.Fatal("a rejected battery must not create a directory")
	}
}

// TestRunGenerateAdapterRejectsTraversal pins the path-traversal rule for
// adapter names: .., separators, and absolute paths fail with the
// errors.Is-matchable ErrPathTraversal sentinel and create nothing.
func TestRunGenerateAdapterRejectsTraversal(t *testing.T) {
	for _, name := range []string{"../evil", "/abs", "a/b", `a\b`, ".."} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			withWorkingDir(t, dir)

			err := runGenerateAdapter([]string{"cache", name})
			if !errors.Is(err, ErrPathTraversal) {
				t.Fatalf("runGenerateAdapter(cache, %q) = %v, want ErrPathTraversal", name, err)
			}

			if _, statErr := os.Stat(filepath.Join(dir, "adapters", "cache")); !os.IsNotExist(statErr) {
				t.Fatal("a rejected adapter name must not create a directory")
			}
		})
	}
}

// TestRunGenerateAdapterObservabilityRejected pins the documented gap:
// observability's Factory returns a struct, so there is no method set to stub.
func TestRunGenerateAdapterObservabilityRejected(t *testing.T) {
	withWorkingDir(t, t.TempDir())

	if err := runGenerateAdapter([]string{"observability", "acme"}); err == nil {
		t.Fatal("want error: observability has no interface to implement")
	}
}

func TestRunGenerateAdapterBadName(t *testing.T) {
	for _, name := range []string{"Memcached", "mem_cached", "1memcached", "mem-cached", ""} {
		t.Run(name, func(t *testing.T) {
			withWorkingDir(t, t.TempDir())

			if err := runGenerateAdapter([]string{"cache", name}); err == nil {
				t.Fatalf("want error for adapter name %q", name)
			}
		})
	}
}

func TestRunGenerateAdapterBadField(t *testing.T) {
	for _, field := range []string{"api_key", "api_key:", ":string", "api_key:complex128", "api-key:string"} {
		t.Run(field, func(t *testing.T) {
			withWorkingDir(t, t.TempDir())

			if err := runGenerateAdapter([]string{"cache", "acme", "--field", field}); err == nil {
				t.Fatalf("want error for --field %q", field)
			}
		})
	}
}

func TestRunGenerateAdapterDuplicateField(t *testing.T) {
	withWorkingDir(t, t.TempDir())

	err := runGenerateAdapter([]string{"cache", "acme", "--field", "api_key:string", "--field", "api_key:int"})
	if err == nil {
		t.Fatal("want error for a repeated option name")
	}
}

func TestRunGenerateAdapterUsage(t *testing.T) {
	withWorkingDir(t, t.TempDir())

	if err := runGenerateAdapter([]string{"cache"}); err == nil {
		t.Fatal("want error for a missing adapter name")
	}

	if err := runGenerateAdapter(nil); err == nil {
		t.Fatal("want error for no arguments")
	}
}

func TestRunGenerateAdapterForce(t *testing.T) {
	dir := generateAdapter(t, "cache", "acme")

	adapterPath := filepath.Join(dir, "adapters", "cache", "acme", "acme.go")

	const handwritten = "package acme\n\n// mine\n"

	if err := os.WriteFile(adapterPath, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runGenerateAdapter([]string{"cache", "acme"})
	if err == nil {
		t.Fatal("want error when the adapter already exists")
	}

	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error should mention --force: %v", err)
	}

	if got := mustRead(t, adapterPath); got != handwritten {
		t.Fatalf("a refused scaffold must not touch the file, got:\n%s", got)
	}

	if err := runGenerateAdapter([]string{"cache", "acme", "--force"}); err != nil {
		t.Fatalf("runGenerateAdapter --force: %v", err)
	}

	if got := mustRead(t, adapterPath); got == handwritten {
		t.Fatal("--force should have overwritten the file")
	}
}

// TestRunGenerateAdapterCollisionIsAllOrNothing proves a collision on a
// later file cannot leave a half-scaffolded package behind: the adapter,
// options and register files are all checked before any is written.
func TestRunGenerateAdapterCollisionIsAllOrNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	pkgDir := filepath.Join(dir, "adapters", "cache", "acme")
	if err := os.MkdirAll(pkgDir, 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	optionsPath := filepath.Join(pkgDir, "options.go")
	if err := os.WriteFile(optionsPath, []byte("package acme\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := runGenerateAdapter([]string{"cache", "acme"}); err == nil {
		t.Fatal("want error when options.go already exists")
	}

	if _, err := os.Stat(filepath.Join(pkgDir, "acme.go")); !os.IsNotExist(err) {
		t.Fatal("acme.go must not be written when options.go collides")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	return string(data)
}

// TestRunGenerateAdapterEveryBattery scaffolds one adapter per table entry and
// checks each generated file is valid, gofmt-clean Go. A malformed signature
// anywhere in the table shows up here rather than in a contributor's build.
func TestRunGenerateAdapterEveryBattery(t *testing.T) {
	for battery := range batterySpecs {
		t.Run(battery, func(t *testing.T) {
			dir := generateAdapter(t, battery, "acme", "--field", "api_key:string", "--field", "timeout:duration")

			readGenerated(t, filepath.Join(dir, "adapters", battery, "acme", "acme.go"))
			readGenerated(t, filepath.Join(dir, "adapters", battery, "acme", "options.go"))
			readGenerated(t, filepath.Join(dir, "adapters", battery, "acme", "register.go"))
		})
	}
}

// TestBatterySpecsCoverEveryBattery guards the table's stated maintenance
// cost: a new core battery package that nobody added to batterySpecs fails
// here.
func TestBatterySpecsCoverEveryBattery(t *testing.T) {
	// The tests run in cmd/zever, so the module root is two levels up and
	// the batteries live under core/.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	entries, err := os.ReadDir(filepath.Join(root, "core"))
	if err != nil {
		t.Fatalf("read core: %v", err)
	}

	// observability is knowingly excluded: its Factory returns a struct.
	// password is knowingly excluded: it has no table entry upstream, and
	// adding a new battery's contract is a separate change from porting the
	// generator.
	excluded := map[string]bool{"observability": true, "password": true}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		battery := entry.Name()

		// A battery package is one whose own <name>.go declares the registry
		// entry point, `func Open(`.
		if !declaresBatteryOpen(t, filepath.Join(root, "core", battery)) {
			continue
		}

		if excluded[battery] {
			if _, listed := batterySpecs[battery]; listed {
				t.Fatalf("battery %q is documented as excluded but is in batterySpecs", battery)
			}

			continue
		}

		if _, listed := batterySpecs[battery]; !listed {
			t.Fatalf("battery %q has a registry but no batterySpecs entry — "+
				"add it so `zever generate adapter %s` works", battery, battery)
		}
	}

	// Every table entry must name a real core battery directory.
	for battery := range batterySpecs {
		if _, err := os.Stat(filepath.Join(root, "core", battery)); err != nil {
			t.Fatalf("batterySpecs entry %q has no core directory: %v", battery, err)
		}
	}
}

func declaresBatteryOpen(t *testing.T, dir string) bool {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}

	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}

		if strings.Contains(mustRead(t, file), "func Open(") {
			return true
		}
	}

	return false
}

// TestGeneratedAdapterScaffoldIsComplete proves a scaffold is the whole
// adapter module shape: stub, options and register files, each valid
// gofmt-clean Go. Real compilation belongs to each adapter module's own
// build (one module per adapters/<battery>/<adapter> since the layout
// split): a cross-module `go build` from this test would need the full
// transitive replace set, so syntax plus the register-content pins below
// are the contract here, not a compiler run.
//
// cache exercises the common shape; router exercises the embedded
// http.Handler and the value-returning methods with no error result.
func TestGeneratedAdapterScaffoldIsComplete(t *testing.T) {
	for _, battery := range []string{"cache", "router"} {
		t.Run(battery, func(t *testing.T) {
			dir := generateAdapter(t, battery, "acme", "--field", "api_key:string", "--field", "timeout:duration")

			readGenerated(t, filepath.Join(dir, "adapters", battery, "acme", "acme.go"))
			readGenerated(t, filepath.Join(dir, "adapters", battery, "acme", "options.go"))

			register := readGenerated(t, filepath.Join(dir, "adapters", battery, "acme", "register.go"))

			if !strings.Contains(register, "func Register()") {
				t.Fatalf("register.go lacks Register:\n%s", register)
			}
		})
	}
}

func TestGoFieldName(t *testing.T) {
	cases := map[string]string{
		"api_key":       "APIKey",
		"timeout":       "Timeout",
		"max_retries":   "MaxRetries",
		"use_tls":       "UseTLS",
		"id":            "ID",
		"base_url":      "BaseURL",
		"ttl":           "TTL",
		"db":            "DB",
		"json_encoding": "JSONEncoding",
		"a_b_c":         "ABC",
	}

	for input, want := range cases {
		if got := goFieldName(input); got != want {
			t.Errorf("goFieldName(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestRunGenerateDispatchesAdapter proves the subcommand is reachable through
// the `zever generate` dispatcher, not only by calling its function directly.
func TestRunGenerateDispatchesAdapter(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerate([]string{"adapter", "cache", "acme"}); err != nil {
		t.Fatalf("runGenerate adapter: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, "adapters", "cache", "acme", "acme.go")); err != nil {
		t.Fatalf("dispatcher did not scaffold the adapter: %v", err)
	}
}
