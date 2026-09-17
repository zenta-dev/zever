package main

import (
	"errors"
	"os"
	"os/exec"
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

	adapter := readGenerated(t, filepath.Join(dir, "cache", "memcached", "memcached.go"))

	for _, fragment := range []string{
		"package memcached",
		`"github.com/zenta-dev/zever/cache"`,
		"func New(_ cache.Options) (cache.Cache, error)",
		"registerAdapters",
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

	options := readGenerated(t, filepath.Join(dir, "cache", "memcached", "options.go"))

	for _, fragment := range []string{
		"package memcached",
		"type Options struct{}",
		"func ParseOptions(_ map[string]any) (Options, error)",
	} {
		if !strings.Contains(options, fragment) {
			t.Fatalf("generated options.go lacks %q:\n%s", fragment, options)
		}
	}

	assertGolden(t, "generate_adapter_cache.golden", []byte(adapter))
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

			src := readGenerated(t, filepath.Join(dir, battery, "acme", "acme.go"))

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

	src := readGenerated(t, filepath.Join(dir, "router", "mux2", "mux2.go"))

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

	options := readGenerated(t, filepath.Join(dir, "storage", "acme", "options.go"))

	for _, fragment := range []string{
		`"time"`,
		`"github.com/zenta-dev/zever/internal/opts"`,
		"APIKey string `json:\"api_key\" toml:\"api_key\" yaml:\"api_key\"`",
		"Timeout time.Duration `json:\"timeout\" toml:\"timeout\" yaml:\"timeout\"`",
		"MaxRetries int `json:\"max_retries\" toml:\"max_retries\" yaml:\"max_retries\"`",
		"Endpoints []string `json:\"endpoints\" toml:\"endpoints\" yaml:\"endpoints\"`",
		"UseTLS bool `json:\"use_tls\" toml:\"use_tls\" yaml:\"use_tls\"`",
		"Extra map[string]any `json:\"extra\" toml:\"extra\" yaml:\"extra\"`",
		"func ParseOptions(m map[string]any) (Options, error)",
		`o.APIKey = opts.String(m, "api_key", "")`,
		`o.Timeout = opts.Duration(m, "timeout", 0)`,
		`o.MaxRetries = opts.Int(m, "max_retries", 0)`,
		`o.Endpoints = opts.StringSlice(m, "endpoints")`,
		`o.UseTLS = opts.Bool(m, "use_tls", false)`,
		`o.Extra = opts.Map(m, "extra")`,
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

	options := readGenerated(t, filepath.Join(dir, "cache", "acme", "options.go"))
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

			if _, statErr := os.Stat(filepath.Join(dir, "cache")); !os.IsNotExist(statErr) {
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

	adapterPath := filepath.Join(dir, "cache", "acme", "acme.go")

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

// TestRunGenerateAdapterCollisionIsAllOrNothing proves a collision on the
// second file cannot leave a half-scaffolded package behind.
func TestRunGenerateAdapterCollisionIsAllOrNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	pkgDir := filepath.Join(dir, "cache", "acme")
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

			readGenerated(t, filepath.Join(dir, battery, "acme", "acme.go"))
			readGenerated(t, filepath.Join(dir, battery, "acme", "options.go"))
		})
	}
}

// TestBatterySpecsCoverEveryBattery guards the table's stated maintenance
// cost: a new battery package that nobody added to batterySpecs fails here.
func TestBatterySpecsCoverEveryBattery(t *testing.T) {
	// The tests run in cmd/zever, so the module root is two levels up.
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read %s: %v", root, err)
	}

	// observability is knowingly excluded: its Factory returns a struct.
	// password is knowingly excluded: it has no table entry upstream, and
	// adding a new battery's contract is a separate change from porting the
	// generator. internal/, cmd/, and scaffolding directories are not
	// batteries at all.
	excluded := map[string]bool{"observability": true, "password": true}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		battery := entry.Name()

		// A battery package is one whose own <name>.go declares the registry
		// entry point, `func Open(`.
		if !declaresBatteryOpen(t, filepath.Join(root, battery)) {
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

	// Every table entry must name a real battery directory.
	for battery := range batterySpecs {
		if _, err := os.Stat(filepath.Join(root, battery)); err != nil {
			t.Fatalf("batterySpecs entry %q has no directory: %v", battery, err)
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

// TestGeneratedAdapterCompiles is the claim the rest of the tests cannot make:
// it scaffolds into this module and runs the real compiler over the result, so
// "a compiling stub" means compiled, not parsed.
func TestGeneratedAdapterCompiles(t *testing.T) {
	if testing.Short() {
		t.Skip("compiling a scaffold is slow")
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skipf("no go toolchain on PATH: %v", err)
	}

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	// The scaffold must land inside this module for the battery import to
	// resolve, so it goes in a temporary directory under the module root.
	sandbox, err := os.MkdirTemp(root, "zever-adapter-build-") //nolint:usetesting // must live under the module root so battery imports resolve
	if err != nil {
		t.Fatalf("mkdtemp under %s: %v", root, err)
	}

	t.Cleanup(func() { _ = os.RemoveAll(sandbox) })

	withWorkingDir(t, sandbox)

	// cache exercises the common shape; router exercises the embedded
	// http.Handler and the value-returning methods with no error result.
	for _, battery := range []string{"cache", "router"} {
		if err := runGenerateAdapter([]string{battery, "acme", "--field", "api_key:string", "--field", "timeout:duration"}); err != nil {
			t.Fatalf("runGenerateAdapter %s: %v", battery, err)
		}
	}

	pattern := "./" + filepath.Base(sandbox) + "/..."

	cmd := exec.CommandContext(t.Context(), goBin, "build", pattern)
	cmd.Dir = root

	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build %s: %v\n%s", pattern, err, out)
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

	if _, err := os.Stat(filepath.Join(dir, "cache", "acme", "acme.go")); err != nil {
		t.Fatalf("dispatcher did not scaffold the adapter: %v", err)
	}
}
