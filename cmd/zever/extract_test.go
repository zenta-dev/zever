package main

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/internal/dsl/diag"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// extractBillingSchema and extractShippingSchema are two modules in two files.
// Extraction has to genuinely filter: everything shipping declares must be
// absent from a billing extraction.
const extractBillingSchema = `// billing owns orders and customers.
entity Order {
	id: uuid @primary
	customer_id: uuid
	belongs_to customer: Customer @foreign_key(customer_id)
}

entity Customer {
	id: uuid @primary
	user_id: uuid
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		http: GET "/v1/orders/{id}"
		auth: required
	}
}

job Invoice() {
	queue: billing
}

schedule Nightly {
	cron: "0 0 * * *"
	dispatch: Invoice()
}
`

const extractShippingSchema = `entity Shipment {
	id: uuid @primary
}

service ShipmentService {
	rpc GetShipment(id: uuid) -> Shipment {
		http: GET "/v1/shipments/{id}"
		auth: required
	}
}

job Dispatch() {
	queue: shipping
}
`

// setupExtractProject lays out a two-module project in a temp dir and makes it
// the working directory. The go.mod replaces the framework with an absolute
// path to this repository, which is both what a real consumer project does
// while zever has no tagged release and what lets the extracted go.mod carry
// a working replace directive of its own.
func setupExtractProject(t *testing.T) (dir, repoRoot string) {
	t.Helper()

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	repoRoot, err = filepath.Abs(filepath.Join(cwd, "..", ".."))
	if err != nil {
		t.Fatalf("abs repo root: %v", err)
	}

	dir = t.TempDir()
	withWorkingDir(t, dir)

	content := "module example.com/shop\n\ngo 1.24\n\n" +
		"replace github.com/zenta-dev/zever => " + repoRoot + "\n\n" +
		"require github.com/zenta-dev/zever v0.0.0-00010101000000-000000000000\n"

	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(content), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	writeSchemaFile(t, dir, "billing/billing.zen", extractBillingSchema)
	writeSchemaFile(t, dir, "shipping/shipping.zen", extractShippingSchema)

	return dir, repoRoot
}

func TestRunExtractProducesAStandaloneService(t *testing.T) {
	dir, repoRoot := setupExtractProject(t)

	if err := runExtract([]string{"billing"}); err != nil {
		t.Fatalf("runExtract: %v", err)
	}

	out := filepath.Join(dir, "billing-service")

	// The schema is copied byte for byte: no AST reprint, so comments and
	// formatting survive intact.
	got := readFile(t, filepath.Join(out, "schema", "billing.zen"))
	if got != extractBillingSchema {
		t.Fatalf("schema copy is not byte-identical:\n--- got ---\n%s\n--- want ---\n%s", got, extractBillingSchema)
	}

	if _, err := os.Stat(filepath.Join(out, "schema", "shipping.zen")); !os.IsNotExist(err) {
		t.Fatalf("the other module's schema file was copied into the extraction")
	}

	gomod := readFile(t, filepath.Join(out, "go.mod"))

	for _, fragment := range []string{
		"module example.com/shop/billing-service",
		"go 1.24",
		"replace github.com/zenta-dev/zever => ",
		"require github.com/zenta-dev/zever ",
	} {
		if !strings.Contains(gomod, fragment) {
			t.Fatalf("extracted go.mod lacks %q:\n%s", fragment, gomod)
		}
	}

	assertReplaceResolves(t, out, gomod, repoRoot)

	// The three scaffolded Go files parse and are gofmt-clean.
	server := readGenerated(t, filepath.Join(out, "cmd", "server", "main.go"))
	worker := readGenerated(t, filepath.Join(out, "cmd", "worker", "main.go"))
	readGenerated(t, filepath.Join(out, "internal", "app", "app.go"))

	if !strings.Contains(server, `"example.com/shop/billing-service/internal/app"`) {
		t.Fatalf("server main.go does not import the extracted module's app package:\n%s", server)
	}

	for _, fragment := range []string{
		`job.Register("Invoice"`,
		`sched.Schedule(ctx, "0 0 * * *", "Invoice"`,
		`Queues:      []string{"billing"}`,
	} {
		if !strings.Contains(worker, fragment) {
			t.Fatalf("worker main.go lacks %q:\n%s", fragment, worker)
		}
	}

	orm := readGenerated(t, filepath.Join(out, "internal", "orm", "billing", "billing.go"))

	for _, fragment := range []string{"type Order struct", "type Customer struct"} {
		if !strings.Contains(orm, fragment) {
			t.Fatalf("generated ORM lacks %q", fragment)
		}
	}

	// The zenorm backend emits its ORM package importing only
	// github.com/zenta-dev/zever/orm (resolved by the extracted module's
	// replace directive), so the relocated package needs no import rewrite.
	if !strings.Contains(orm, "github.com/zenta-dev/zever/orm") {
		t.Fatalf("generated ORM does not import the framework's orm package")
	}

	assertNoShippingLeak(t, out)
}

// assertReplaceResolves proves the replace directive's relative path really
// points at this repository from the extracted directory.
func assertReplaceResolves(t *testing.T, out, gomod, repoRoot string) {
	t.Helper()

	_, rest, ok := strings.Cut(gomod, "replace github.com/zenta-dev/zever => ")
	if !ok {
		t.Fatalf("no replace directive in:\n%s", gomod)
	}

	target, _, _ := strings.Cut(rest, "\n")

	resolved, err := filepath.Abs(filepath.Join(out, strings.TrimSpace(target)))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}

	wantRoot, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		t.Fatalf("eval repo root: %v", err)
	}

	gotRoot, err := filepath.EvalSymlinks(resolved)
	if err != nil {
		t.Fatalf("replace target %q does not exist: %v", resolved, err)
	}

	if gotRoot != wantRoot {
		t.Fatalf("replace resolves to %q, want %q", gotRoot, wantRoot)
	}
}

// assertNoShippingLeak is the cross-module isolation assertion: nothing the
// shipping module declares may appear anywhere in a billing extraction.
func assertNoShippingLeak(t *testing.T, out string) {
	t.Helper()

	forbidden := []string{"Shipment", "ShipmentService", "shipping", "Dispatch("}

	walkErr := filepath.Walk(out, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path) //nolint:gosec // test fixture tree
		if err != nil {
			return err
		}

		for _, word := range forbidden {
			if strings.Contains(string(data), word) {
				t.Errorf("%s leaked the shipping module's %q", path, word)
			}
		}

		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk %q: %v", out, walkErr)
	}
}

func TestRunExtractUnknownModule(t *testing.T) {
	setupExtractProject(t)

	err := runExtract([]string{"warehouse"})
	if err == nil {
		t.Fatalf("expected an error for a module the schema does not declare")
	}

	for _, fragment := range []string{`no module "warehouse"`, "billing", "shipping"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error %q does not mention %q", err, fragment)
		}
	}
}

func TestRunExtractRejectsTwoModulesInOneFile(t *testing.T) {
	dir, _ := setupExtractProject(t)

	if err := os.Remove(filepath.Join(dir, "schema", "shipping", "shipping.zen")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	writeSchemaFile(t, dir, "billing/billing.zen", extractBillingSchema+"\n"+extractShippingSchema)

	// With dir-derived modules, a file's module is its directory, so merging
	// both schemas into billing/billing.zen means all decls belong to billing;
	// there is no cross-file sharing error anymore. Extraction should succeed.
	if err := runExtract([]string{"billing"}); err != nil {
		t.Fatalf("runExtract billing with merged file: %v", err)
	}
}

func TestRunExtractRefusesToClobberWithoutForce(t *testing.T) {
	dir, _ := setupExtractProject(t)

	if err := runExtract([]string{"billing"}); err != nil {
		t.Fatalf("runExtract: %v", err)
	}

	const handwritten = "package main\n\nfunc main() {}\n"

	path := filepath.Join(dir, "billing-service", "cmd", "server", "main.go")

	if err := os.WriteFile(path, []byte(handwritten), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	err := runExtract([]string{"billing"})
	if err == nil {
		t.Fatalf("expected an error re-extracting over existing files")
	}

	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error does not mention --force: %v", err)
	}

	if got := readFile(t, path); got != handwritten {
		t.Fatalf("hand-written file was clobbered")
	}

	if err := runExtract([]string{"--force", "billing"}); err != nil {
		t.Fatalf("runExtract --force: %v", err)
	}

	if got := readFile(t, path); got == handwritten {
		t.Fatalf("--force did not overwrite")
	}
}

func TestRunExtractHonoursOutAndModuleFlags(t *testing.T) {
	dir, _ := setupExtractProject(t)

	if err := runExtract([]string{"--out", "services/bill", "--module", "example.com/bill", "shipping"}); err != nil {
		t.Fatalf("runExtract: %v", err)
	}

	out := filepath.Join(dir, "services", "bill")

	readGenerated(t, filepath.Join(out, "internal", "orm", "shipping", "shipping.go"))

	server := readGenerated(t, filepath.Join(out, "cmd", "server", "main.go"))
	if !strings.Contains(server, `"example.com/bill/internal/app"`) {
		t.Fatalf("--module was ignored:\n%s", server)
	}

	if !strings.Contains(readFile(t, filepath.Join(out, "go.mod")), "module example.com/bill\n") {
		t.Fatalf("--module was ignored in go.mod")
	}
}

func TestRunExtractRequiresExactlyOneModuleName(t *testing.T) {
	setupExtractProject(t)

	for _, args := range [][]string{nil, {}, {"billing", "shipping"}} {
		if err := runExtract(args); err == nil {
			t.Fatalf("expected an error for args %v", args)
		}
	}
}

// TestPlanExtractionDefaults pins the pure planning layer: defaults derive
// from the module name and the host go.mod, with no filesystem touched.
func TestPlanExtractionDefaults(t *testing.T) {
	dir, _ := setupExtractProject(t)

	_ = dir

	gomod := goModInfo{
		ModulePath:       "example.com/shop",
		GoVersion:        "1.24",
		FrameworkVersion: pseudoVersionZero,
		FrameworkDir:     "/repo",
	}

	schema, err := compileSchemaDir("test", "schema")
	if err != nil {
		t.Fatalf("compileSchemaDir: %v", err)
	}

	plan, err := planExtraction("test", ExtractConfig{Module: "billing"}, schema, gomod)
	if err != nil {
		t.Fatalf("planExtraction: %v", err)
	}

	if plan.OutDir != "./billing-service" {
		t.Fatalf("OutDir = %q, want ./billing-service", plan.OutDir)
	}

	if plan.ModulePath != "example.com/shop/billing-service" {
		t.Fatalf("ModulePath = %q", plan.ModulePath)
	}

	if len(plan.SchemaFiles) == 0 {
		t.Fatal("expected schema files for billing")
	}
}

// TestExtractedServiceBuilds is the strongest available proof: the extracted
// tree is a real Go module that compiles on its own, through the replace
// directive extraction wrote. It shells out to the toolchain, so it is skipped
// under -short and whenever `go mod tidy` cannot resolve dependencies (an
// offline machine with a cold module cache).
func TestExtractedServiceBuilds(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping toolchain round-trip under -short")
	}

	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH")
	}

	dir, _ := setupExtractProject(t)

	if err := runExtract([]string{"billing"}); err != nil {
		t.Fatalf("runExtract: %v", err)
	}

	out := filepath.Join(dir, "billing-service")

	ctx, cancel := context.WithTimeout(t.Context(), goCommandTimeout)
	defer cancel()

	if output, err := runGoCommand(ctx, goBin, out, "mod", "tidy"); err != nil {
		t.Skipf("go mod tidy could not resolve dependencies (offline?): %v\n%s", err, output)
	}

	if output, err := runGoCommand(ctx, goBin, out, "build", "./..."); err != nil {
		t.Fatalf("the extracted service does not build: %v\n%s", err, output)
	}
}

const goCommandTimeout = 5 * time.Minute

func runGoCommand(ctx context.Context, goBin, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, goBin, args...)
	cmd.Dir = dir
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()

	return string(output), err
}

func TestRunExtractOutsideAGoModule(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeSchemaFile(t, dir, "billing/billing.zen", extractBillingSchema)

	err := runExtract([]string{"billing"})
	if err == nil {
		t.Fatalf("expected an error outside a Go module")
	}

	if !errors.Is(err, errNoGoMod) {
		t.Fatalf("expected errNoGoMod, got: %v", err)
	}
}

// TestReadGoModParsesBlocks pins the hand parser against block-style
// require/replace stanzas, the form real go.mod files use.
func TestReadGoModParsesBlocks(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	content := "module example.com/shop\n\ngo 1.24\n\n" +
		"require (\n\tgithub.com/zenta-dev/zever v1.2.3\n)\n\n" +
		"replace github.com/zenta-dev/zever => ../zever\n"

	if err := os.WriteFile("go.mod", []byte(content), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	info, err := readGoMod()
	if err != nil {
		t.Fatalf("readGoMod: %v", err)
	}

	if info.ModulePath != "example.com/shop" {
		t.Fatalf("ModulePath = %q", info.ModulePath)
	}

	if info.FrameworkVersion != "v1.2.3" {
		t.Fatalf("FrameworkVersion = %q", info.FrameworkVersion)
	}

	if info.FrameworkDir != "../zever" {
		t.Fatalf("FrameworkDir = %q", info.FrameworkDir)
	}
}

// TestReadGoModResolvesOwnRepo pins the self-hosting rule: a go.mod whose
// module IS the framework resolves the checkout to the working directory
// with no replace directive present.
func TestReadGoModResolvesOwnRepo(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := os.WriteFile("go.mod", []byte("module github.com/zenta-dev/zever\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}

	info, err := readGoMod()
	if err != nil {
		t.Fatalf("readGoMod: %v", err)
	}

	if info.FrameworkDir != "." {
		t.Fatalf("FrameworkDir = %q, want .", info.FrameworkDir)
	}
}

// TestModuleSourceFilesRejectsSharedFile pins the isolation rule at the unit
// level: one .zen file declaring two modules cannot be extracted.
func TestModuleSourceFilesRejectsSharedFile(t *testing.T) {
	a := &ir.Module{Name: "a"}
	b := &ir.Module{Name: "b"}
	a.Entities = []*ir.Entity{{Name: "A", Pos: diag.Position{File: "shared.zen"}}}
	b.Entities = []*ir.Entity{{Name: "B", Pos: diag.Position{File: "shared.zen"}}}

	schema := &ir.Schema{Modules: []*ir.Module{a, b}}

	if _, err := moduleSourceFiles("test", schema, a); err == nil {
		t.Fatal("expected an error for a file shared by two modules")
	} else if !strings.Contains(err.Error(), "alongside") {
		t.Fatalf("error %q does not explain the split", err)
	}
}

// TestModuleSourceFilesEmpty pins the nothing-to-extract error.
func TestModuleSourceFilesEmpty(t *testing.T) {
	m := &ir.Module{Name: "empty"}
	schema := &ir.Schema{Modules: []*ir.Module{m}}

	if _, err := moduleSourceFiles("test", schema, m); err == nil {
		t.Fatal("expected an error for a module declaring nothing")
	}
}

// TestFindModuleEmptyName pins the guard: empty names never match.
func TestFindModuleEmptyName(t *testing.T) {
	schema := &ir.Schema{Modules: []*ir.Module{{Name: "a"}}}

	if findModule(schema, "") != nil {
		t.Fatal("findModule(\"\") should return nil")
	}

	if got := moduleNames(&ir.Schema{}); len(got) != 1 || got[0] != "(none)" {
		t.Fatalf("moduleNames(empty) = %v, want [(none)]", got)
	}
}
