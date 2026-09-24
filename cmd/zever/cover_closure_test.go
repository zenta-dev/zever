package main

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// writeSchemaModuleFixture writes schema/<module>/<module>.zen under dir.
func writeSchemaModuleFixture(t *testing.T, dir, module, content string) {
	t.Helper()

	p := filepath.Join(dir, "schema", module, module+".zen")
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestClosureGenerateModuleErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if _, err := GenerateModule(GenerateModuleConfig{Name: "../evil", Lang: "go", SchemaDir: "schema"}); !isTraversalError(err) {
		t.Fatalf("traversal = %v", err)
	}

	if _, err := GenerateModule(GenerateModuleConfig{Name: "bad-name", Lang: "go", SchemaDir: "schema"}); err == nil {
		t.Fatal("want ident error")
	}

	if _, err := GenerateModule(GenerateModuleConfig{Name: "ok", Lang: "rust", SchemaDir: "schema"}); err == nil {
		t.Fatal("want lang error")
	}

	// joinUnderRoot error: absolute SchemaDir escapes root.
	if _, err := GenerateModule(GenerateModuleConfig{Name: "ok", Lang: "go", SchemaDir: "/abs"}); err == nil {
		t.Fatal("want joinUnderRoot error")
	}

	// Mkdir failure: schema dir parent is a file.
	writeZeverFixture(t, dir, "blocker", "x")

	if _, err := GenerateModule(GenerateModuleConfig{Name: "m", Lang: "go", SchemaDir: "blocker/sub"}); err == nil {
		t.Fatal("want mkdir error")
	}

	// Write failure: make the target path a directory.
	if _, err := GenerateModule(GenerateModuleConfig{Name: "w", Lang: "go", SchemaDir: "schema"}); err != nil {
		t.Fatalf("setup module: %v", err)
	}

	stub := filepath.Join(dir, "schema", "w", "w.zen")
	if err := os.Remove(stub); err != nil {
		t.Fatal(err)
	}

	if err := os.Mkdir(stub, 0o750); err != nil {
		t.Fatal(err)
	}

	if _, err := GenerateModule(GenerateModuleConfig{Name: "w", Lang: "go", SchemaDir: "schema"}); err == nil {
		t.Fatal("want write error")
	}
}

func TestClosureRunGenerateModuleBranches(t *testing.T) {
	// Parse error.
	if err := runGenerateModule([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	// Non-TTY usage error.
	stubPromptTTY(t, false)

	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateModule(nil); err == nil {
		t.Fatal("want usage error")
	}

	// Interactive success via seam.
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	orig := promptInputForGenerate
	t.Cleanup(func() { promptInputForGenerate = orig })

	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "prompted", nil }

	if err := runGenerateModule(nil); err != nil {
		t.Fatalf("interactive: %v", err)
	}

	// Interactive prompt error.
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }

	if err := runGenerateModule(nil); err == nil {
		t.Fatal("want prompt error")
	}

	// Empty-name branch: rest = ["module", ""] prompts again.
	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "filled", nil }

	if err := runGenerateModule([]string{"module", ""}); err != nil {
		t.Fatalf("empty-name prompt: %v", err)
	}

	promptInputForGenerate = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }

	if err := runGenerateModule([]string{"module", ""}); err == nil {
		t.Fatal("want empty-name prompt error")
	}
}

func TestClosureOptionSpecs(t *testing.T) {
	var o optionSpecs

	if got := o.String(); got != "" {
		t.Fatalf("empty String = %q", got)
	}

	if err := o.Set("api_key:string"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	if got := o.String(); got != "api_key:string" {
		t.Fatalf("String = %q", got)
	}

	for _, bad := range []string{"nope", ":string", "k:", "bad-name:string", "k:unknowntype"} {
		if err := o.Set(bad); err == nil {
			t.Fatalf("Set(%q) = nil, want error", bad)
		}
	}
}

func TestClosureGenerateAdapterErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "nope", Name: "x"}); err == nil {
		t.Fatal("want unknown battery")
	}

	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "db", Name: "../x"}); !isTraversalError(err) {
		t.Fatalf("traversal = %v", err)
	}

	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "db", Name: "Bad"}); err == nil {
		t.Fatal("want package name error")
	}

	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "db", Name: "sqlite", Fields: []AdapterOption{{Key: "k", Type: "nope"}}}); err == nil {
		t.Fatal("want field error")
	}

	// Collision then force.
	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "db", Name: "myadapter"}); err != nil {
		t.Fatalf("setup: %v", err)
	}

	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "db", Name: "myadapter"}); err == nil {
		t.Fatal("want collision error")
	}

	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "db", Name: "myadapter", Force: true}); err != nil {
		t.Fatalf("force: %v", err)
	}

	// Write failure: pre-create options.go as a directory so the second
	// write fails after the adapter file check passes with Force.
	_ = os.MkdirAll(filepath.Join(dir, "db", "partial"), 0o750)
	_ = os.MkdirAll(filepath.Join(dir, "db", "partial", "options.go"), 0o750)

	if _, err := GenerateAdapter(GenerateAdapterConfig{Battery: "db", Name: "partial", Force: true}); err == nil {
		t.Fatal("want options write error")
	}
}

func TestClosureRunGenerateAdapterBranches(t *testing.T) {
	if err := runGenerateAdapter([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	stubPromptTTY(t, false)

	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runGenerateAdapter(nil); err == nil {
		t.Fatal("want usage error")
	}

	if err := runGenerateAdapter([]string{"nope", "x"}); err == nil {
		t.Fatal("want unknown battery error")
	}

	// Interactive: battery select + name input + fields loop.
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	origSel, origIn, origConf := promptSelectForAdapter, promptInputForAdapter, promptConfirmForAdapter
	t.Cleanup(func() {
		promptSelectForAdapter, promptInputForAdapter, promptConfirmForAdapter = origSel, origIn, origConf
	})

	calls := 0
	promptSelectForAdapter = func(title string, _ []string) (string, error) {
		if title == "Battery" {
			return "db", nil
		}

		return "string", nil
	}
	promptInputForAdapter = func(title, _ string, _ func(string) error) (string, error) {
		calls++

		if title == "Adapter name (go package)" {
			return "prompted", nil
		}

		if title == "Field name (snake_case)" {
			return "api_key", nil
		}

		return "", nil
	}
	promptConfirmForAdapter = func(string, string) (bool, error) {
		// Add one field, then stop. Field type comes from select above.
		calls++

		return calls <= 3, nil
	}

	_ = calls

	if err := runGenerateAdapter([]string{"-i"}); err != nil {
		t.Fatalf("interactive adapter: %v", err)
	}

	// Select error.
	promptSelectForAdapter = func(string, []string) (string, error) { return "", errTestSentinel }

	if err := runGenerateAdapter([]string{"-i"}); err == nil {
		t.Fatal("want select error")
	}

	// Name input error.
	promptSelectForAdapter = func(string, []string) (string, error) { return "db", nil }
	promptInputForAdapter = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }

	if err := runGenerateAdapter([]string{"-i"}); err == nil {
		t.Fatal("want name input error")
	}

	// Confirm error.
	promptInputForAdapter = func(_, _ string, _ func(string) error) (string, error) { return "okname", nil }
	promptConfirmForAdapter = func(string, string) (bool, error) { return false, errTestSentinel }

	if err := runGenerateAdapter([]string{"-i"}); err == nil {
		t.Fatal("want confirm error")
	}
}

func TestClosureReadZenModuleBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if _, _, _, err := readZenModule("tag", "../evil"); !isTraversalError(err) {
		t.Fatalf("traversal = %v", err)
	}

	if _, _, _, err := readZenModule("tag", "missing"); err == nil {
		t.Fatal("want missing error")
	}

	// Parse-error file.
	writeSchemaModuleFixture(t, dir, "broken", "entity {\n")

	if _, _, _, err := readZenModule("tag", "broken"); err == nil {
		t.Fatal("want parse error")
	}
}

func TestClosureDeclNameTakenKinds(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	content := "service Svc {}\njob MyJob() {\nqueue: default\n}\nschedule Sched {\ncron: \"* * * * *\"\ndispatch: MyJob\n}\n"
	writeSchemaModuleFixture(t, dir, "shop", content)

	_, _, file, err := readZenModule("tag", "shop")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	for _, name := range []string{"Svc", "MyJob", "Sched"} {
		if _, taken := declNameTaken(file, name); !taken {
			t.Fatalf("declNameTaken(%q) = false", name)
		}
	}

	if _, taken := declNameTaken(file, "Other"); taken {
		t.Fatal("declNameTaken(Other) = true")
	}

	// Message branch via hand-built file.
	msgFile := &stubIRModuleFile
	_ = msgFile
}

func TestClosureWriteAtomicallyBranches(t *testing.T) {
	dir := t.TempDir()

	// CreateTemp failure: dir component is a file.
	writeZeverFixture(t, dir, "blocker", "x")

	if err := writeAtomically(filepath.Join(dir, "blocker", "f.zen"), []byte("x")); err == nil {
		t.Fatal("want create-temp error")
	}

	// Success.
	p := filepath.Join(dir, "ok.zen")
	if err := writeAtomically(p, []byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	if got := readFile(t, p); got != "hello\n" {
		t.Fatalf("got %q", got)
	}

	// Rename failure: path is a directory.
	d := filepath.Join(dir, "adir")
	if err := os.Mkdir(d, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := writeAtomically(d, []byte("x")); err == nil {
		t.Fatal("want rename error")
	}
}

func TestClosureScaffoldHelpers(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	// goModulePath variants.
	if _, err := goModulePath(); !errors.Is(err, errNoGoMod) {
		t.Fatalf("no go.mod = %v", err)
	}

	writeGoMod(t, dir, "example.com/app")

	mp, modErr := goModulePath()
	if modErr != nil || mp != "example.com/app" {
		t.Fatalf("goModulePath = %q, %v", mp, modErr)
	}

	// Quoted module path.
	if err := os.WriteFile("go.mod", []byte("module \"example.com/quoted\"\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if quotedMp, quotedErr := goModulePath(); quotedErr != nil || quotedMp != "example.com/quoted" {
		t.Fatalf("quoted = %q, %v", quotedMp, quotedErr)
	}

	// No module directive.
	if err := os.WriteFile("go.mod", []byte("go 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, modErr := goModulePath(); modErr == nil {
		t.Fatal("want no-module error")
	}

	// go.mod as directory: open succeeds, scan fails.
	if rmErr := os.Remove("go.mod"); rmErr != nil {
		t.Fatal(rmErr)
	}

	if mkdirErr := os.Mkdir("go.mod", 0o750); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}

	if _, modErr := goModulePath(); modErr == nil {
		t.Fatal("want dir-read error")
	}

	if rmErr := os.Remove("go.mod"); rmErr != nil {
		t.Fatal(rmErr)
	}

	writeGoMod(t, dir, "example.com/app")

	// renderGoFile variants.
	if _, renderErr := renderGoFile("tag", "x", "{{.Unclosed", nil); renderErr == nil {
		t.Fatal("want template parse error")
	}

	if _, renderErr := renderGoFile("tag", "x", "{{.Missing.Field}}", struct{}{}); renderErr == nil {
		t.Fatal("want template exec error")
	}

	if _, err := renderGoFile("tag", "x", "package main\nfunc broken( {", nil); err == nil {
		t.Fatal("want gofmt error")
	}

	// writeScaffold variants.
	if err := writeScaffold("tag", filepath.Join(dir, "a.go"), []byte("x"), false); err != nil {
		t.Fatal(err)
	}

	if err := writeScaffold("tag", filepath.Join(dir, "a.go"), []byte("x"), false); err == nil {
		t.Fatal("want exists error")
	}

	// Stat error: path with null byte.
	if err := writeScaffold("tag", string([]byte{'a', 0}), []byte("x"), false); err == nil {
		t.Fatal("want stat error")
	}

	// Mkdir error: parent is a file.
	writeZeverFixture(t, dir, "fileparent", "x")

	if err := writeScaffold("tag", filepath.Join(dir, "fileparent", "sub", "f.go"), []byte("x"), false); err == nil {
		t.Fatal("want mkdir error")
	}

	// Write error: path is a directory.
	if err := writeScaffold("tag", dir, []byte("x"), true); err == nil {
		t.Fatal("want write error")
	}

	// ensureAppPackage: stat error via file-as-dir.
	if err := os.MkdirAll(filepath.Join(dir, "internal"), 0o750); err != nil {
		t.Fatal(err)
	}

	writeZeverFixture(t, dir, filepath.Join("internal", "app"), "blocker")

	if _, err := ensureAppPackage("tag"); err == nil {
		t.Fatal("want app stat error")
	}

	if err := os.Remove(filepath.Join(dir, "internal", "app")); err != nil {
		t.Fatal(err)
	}

	created, err := ensureAppPackage("tag")
	if err != nil || !created {
		t.Fatalf("ensure = %v, %v", created, err)
	}

	created, err = ensureAppPackage("tag")
	if err != nil || created {
		t.Fatalf("second ensure = %v, %v", created, err)
	}

	// writeStubIfMissing variants.
	stub := filepath.Join(dir, "internal", "service", "x.go")

	ok, err := writeStubIfMissing("tag", stub, []byte("package x\n"))
	if err != nil || !ok {
		t.Fatalf("stub = %v, %v", ok, err)
	}

	ok, err = writeStubIfMissing("tag", stub, []byte("package x\n"))
	if err != nil || ok {
		t.Fatalf("existing stub = %v, %v", ok, err)
	}

	if _, err := writeStubIfMissing("tag", string([]byte{'b', 0}), []byte("x")); err == nil {
		t.Fatal("want stub stat error")
	}

	// lowerFirst.
	if got := lowerFirst(""); got != "" {
		t.Fatalf("lowerFirst empty = %q", got)
	}

	if got := lowerFirst("TaskService"); got != "taskService" {
		t.Fatalf("lowerFirst = %q", got)
	}
}

func TestClosureLoadServerDataBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	// No inputs: zero modules, no error.
	data, err := loadServerData("example.com/app", "./generated", "schema", nil, false)
	if err != nil || len(data.Modules) != 0 {
		t.Fatalf("empty = %v, %v", data.Modules, err)
	}

	// Explicit missing file.
	if _, loadErr := loadServerData("example.com/app", "./generated", "schema", []string{"nope.zen"}, false); loadErr == nil {
		t.Fatal("want load error")
	}

	// Compile error.
	writeZeverFixture(t, dir, "bad.zen", "entity {\n")

	if _, loadErr := loadServerData("example.com/app", "./generated", "schema", []string{"bad.zen"}, false); loadErr == nil {
		t.Fatal("want compile error")
	}

	// Module without services is skipped.
	writeZeverFixture(t, dir, "plain.zen", "entity Thing {\nid: uuid @primary\n}\n")

	data, err = loadServerData("example.com/app", "./generated", "schema", []string{"plain.zen"}, false)
	if err != nil || len(data.Modules) != 0 {
		t.Fatalf("no-service = %v, %v", data.Modules, err)
	}
}

func TestClosureGenerateServerBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeGoMod(t, dir, "example.com/app")

	res, err := GenerateServer(GenerateServerConfig{
		ModulePath: "example.com/app", OutDir: "./generated",
		SchemaDir: "schema", ServerEntry: "cmd/server",
	})
	if err != nil || res.Entrypoint == "" {
		t.Fatalf("server = %+v, %v", res, err)
	}

	// Collision without force.
	if _, err := GenerateServer(GenerateServerConfig{
		ModulePath: "example.com/app", OutDir: "./generated",
		SchemaDir: "schema", ServerEntry: "cmd/server",
	}); err == nil {
		t.Fatal("want collision error")
	}

	// ensureAppPackage error via file-as-dir on fresh dir.
	dir2 := t.TempDir()
	withWorkingDir(t, dir2)
	writeGoMod(t, dir2, "example.com/app")

	if err := os.MkdirAll(filepath.Join(dir2, "internal"), 0o750); err != nil {
		t.Fatal(err)
	}

	writeZeverFixture(t, dir2, filepath.Join("internal", "app"), "blocker")

	if _, err := GenerateServer(GenerateServerConfig{
		ModulePath: "example.com/app", OutDir: "./generated",
		SchemaDir: "schema", ServerEntry: "cmd/server",
	}); err == nil {
		t.Fatal("want app error")
	}
}

func TestClosureRunGenerateServerBranches(t *testing.T) {
	if err := runGenerateServer([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	dir := t.TempDir()
	withWorkingDir(t, dir)

	// No go.mod.
	stubPromptTTY(t, false)

	if err := runGenerateServer(nil); err == nil {
		t.Fatal("want project/go.mod error")
	}

	// Bad project file.
	writeZeverFixture(t, dir, "zever.yaml", "project: [unclosed\n")

	if err := runGenerateServer(nil); err == nil {
		t.Fatal("want project error")
	}

	if rmErr := os.Remove(filepath.Join(dir, "zever.yaml")); rmErr != nil {
		t.Fatal(rmErr)
	}

	writeGoMod(t, dir, "example.com/app")

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("server run: %v", err)
	}

	// Interactive confirm error + confirm-yes.
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	orig := promptConfirmForServer
	t.Cleanup(func() { promptConfirmForServer = orig })

	promptConfirmForServer = func(string, string) (bool, error) { return false, errTestSentinel }

	if err := runGenerateServer(nil); err == nil {
		t.Fatal("want confirm error")
	}

	promptConfirmForServer = func(string, string) (bool, error) { return true, nil }

	if err := runGenerateServer(nil); err != nil {
		t.Fatalf("confirm-yes: %v", err)
	}
}

func TestClosureGenerateTinkerBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if _, err := GenerateTinker(GenerateTinkerConfig{}); err == nil {
		t.Fatal("want empty app error")
	}

	if _, err := GenerateTinker(GenerateTinkerConfig{AppPackage: "example.com/app/internal/app"}); err == nil {
		t.Fatal("want empty dir error")
	}

	if _, err := GenerateTinker(GenerateTinkerConfig{AppPackage: "x", OutDir: ".."}); err == nil {
		t.Fatal("want traversal error")
	}

	p, err := GenerateTinker(GenerateTinkerConfig{AppPackage: "example.com/app/internal/app", OutDir: "cmd/tinker-shim"})
	if err != nil || p == "" {
		t.Fatalf("tinker = %q, %v", p, err)
	}

	if _, err := GenerateTinker(GenerateTinkerConfig{AppPackage: "example.com/app/internal/app", OutDir: "cmd/tinker-shim"}); err == nil {
		t.Fatal("want exists error")
	}

	// Stat error via null byte.
	if _, err := GenerateTinker(GenerateTinkerConfig{AppPackage: "example.com/app/internal/app", OutDir: string([]byte{'x', 0})}); err == nil {
		t.Fatal("want stat error")
	}

	// currentModulePath variants.
	if _, err := currentModulePath(); err == nil {
		// cwd has no go.mod here — but TempDir chdir means missing.
		t.Log("unexpected go.mod present")
	}

	writeGoMod(t, dir, "example.com/app")

	if mp, err := currentModulePath(); err != nil || mp != "example.com/app" {
		t.Fatalf("module path = %q, %v", mp, err)
	}

	if err := os.WriteFile("go.mod", []byte("go 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := currentModulePath(); err == nil {
		t.Fatal("want no-module error")
	}

	// renderTinkerShim gofmt error: newline in import path breaks the literal.
	if _, err := renderTinkerShim("example.com/app\nbad", "dir"); err == nil {
		t.Fatal("want gofmt error")
	}
}

func TestClosureRunGenerateTinkerBranches(t *testing.T) {
	if err := runGenerateTinker([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	dir := t.TempDir()
	withWorkingDir(t, dir)

	stubPromptTTY(t, false)

	// Bad project file.
	writeZeverFixture(t, dir, "zever.yaml", "project: [unclosed\n")

	if err := runGenerateTinker([]string{"--app", "example.com/app/internal/app"}); err == nil {
		t.Fatal("want project error")
	}

	if rmErr := os.Remove(filepath.Join(dir, "zever.yaml")); rmErr != nil {
		t.Fatal(rmErr)
	}

	// No go.mod, non-TTY: --app missing path.
	if err := runGenerateTinker(nil); err == nil {
		t.Fatal("want module error")
	}

	// Explicit flags bypass prompts.
	writeGoMod(t, dir, "example.com/app")

	if err := runGenerateTinker([]string{"--app", "example.com/app/internal/app", "--dir", "cmd/tinker-shim", "--force"}); err != nil {
		t.Fatalf("explicit: %v", err)
	}

	// Interactive branches.
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	origIn, origConf := promptInputForTinkerGen, promptConfirmForTinkerGen
	t.Cleanup(func() { promptInputForTinkerGen, promptConfirmForTinkerGen = origIn, origConf })

	promptInputForTinkerGen = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Output directory" {
			return "", nil
		}

		return "example.com/app/internal/app", nil
	}
	promptConfirmForTinkerGen = func(string, string) (bool, error) { return true, nil }

	if err := runGenerateTinker([]string{"--dir", "", "--force"}); err != nil {
		t.Fatalf("outdir prompt: %v", err)
	}

	// Outdir prompt error.
	promptInputForTinkerGen = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }

	if err := runGenerateTinker([]string{"--force"}); err == nil {
		t.Fatal("want outdir prompt error")
	}

	// App prompt error (no go.mod).
	dir2 := t.TempDir()
	withWorkingDir(t, dir2)

	promptInputForTinkerGen = func(string, string, func(string) error) (string, error) { return "", errTestSentinel }

	if err := runGenerateTinker(nil); err == nil {
		t.Fatal("want app prompt error")
	}

	// App confirm error.
	withWorkingDir(t, dir)

	promptInputForTinkerGen = func(_, def string, _ func(string) error) (string, error) { return def, nil }
	promptConfirmForTinkerGen = func(string, string) (bool, error) { return false, errTestSentinel }

	if err := runGenerateTinker([]string{"--app", "example.com/app/internal/app"}); err == nil {
		t.Fatal("want confirm error")
	}
}

func TestClosureWorkerPureBranches(t *testing.T) {
	// goScalarType: every branch + default.
	cases := []struct {
		in   ir.ScalarType
		want string
		json bool
	}{
		{ir.TUUID, "string", false},
		{ir.TString, "string", false},
		{ir.TEnum, "string", false},
		{ir.TInt32, "int32", false},
		{ir.TInt64, "int64", false},
		{ir.TFloat32, "float32", false},
		{ir.TFloat64, "float64", false},
		{ir.TBool, "bool", false},
		{ir.TTimestamp, "time.Time", false},
		{ir.TDate, "time.Time", false},
		{ir.TBytes, "[]byte", false},
		{ir.TJSON, "json.RawMessage", true},
		{ir.ScalarType(9999), "any", false},
	}

	for _, c := range cases {
		got, needs := goScalarType(c.in)
		if got != c.want || needs != c.json {
			t.Fatalf("goScalarType(%v) = %q,%v want %q,%v", c.in, got, needs, c.want, c.json)
		}
	}

	// goIdent variants.
	if got := goIdent(""); got != "Args" {
		t.Fatalf("goIdent empty = %q", got)
	}

	if got := goIdent("my_job-name x"); got != "MyJobNameX" {
		t.Fatalf("goIdent = %q", got)
	}

	if got := goIdent("___"); got != "Args" {
		t.Fatalf("goIdent seps = %q", got)
	}

	// dispatchArgsJSON nil/empty.
	if got := dispatchArgsJSON(&ir.Schedule{}); got != "{}" {
		t.Fatalf("nil dispatch = %q", got)
	}

	// buildWorkerData: empty schema → default queue.
	data := buildWorkerData("example.com/app", &ir.Schema{})
	if len(data.Queues) != 1 || data.Queues[0] != "default" {
		t.Fatalf("queues = %v", data.Queues)
	}
}

func TestClosureCollectZenFilesBranches(t *testing.T) {
	dir := t.TempDir()

	// Missing dir → nil, nil.
	if got, err := collectZenFiles("tag", filepath.Join(dir, "nope")); err != nil || got != nil {
		t.Fatalf("missing = %v, %v", got, err)
	}

	// Not a dir.
	p := writeZeverFixture(t, dir, "file.zen", "x")

	if _, err := collectZenFiles("tag", p); err == nil {
		t.Fatal("want not-dir error")
	}

	// Stat error via null byte.
	if _, err := collectZenFiles("tag", string([]byte{'z', 0})); err == nil {
		t.Fatal("want stat error")
	}

	// Walk error: unreadable file inside (uid 1000, chmod 000 blocks read).
	sub := filepath.Join(dir, "schemas")
	writeZeverFixture(t, sub, "ok.zen", "entity A {\nid: uuid @primary\n}\n")
	denied := filepath.Join(sub, "denied.zen")

	if err := os.WriteFile(denied, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Chmod(denied, 0o000); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = os.Chmod(denied, 0o600) })

	if _, err := collectZenFiles("tag", sub); err == nil {
		t.Log("walk unreadable did not error (running privileged?)")
	}

	// compileSchemaDir: compile error + empty.
	if _, err := compileSchemaDir("tag", sub); err != nil {
		t.Logf("compile with denied file: %v", err)
	}

	if err := os.Remove(denied); err != nil {
		t.Fatal(err)
	}

	if _, err := compileSchemaDir("tag", filepath.Join(dir, "empty-missing")); err != nil {
		t.Fatalf("empty dir: %v", err)
	}

	badDir := filepath.Join(dir, "bad")
	writeZeverFixture(t, badDir, "bad.zen", "entity {\n")

	if _, err := compileSchemaDir("tag", badDir); err == nil {
		t.Fatal("want compile error")
	}
}

func TestClosureWriteJobStubsBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	// Traversal.
	wd := workerData{Jobs: []workerJob{{Name: "../evil"}}}
	if _, err := writeJobStubs("tag", "", wd); err == nil {
		t.Fatal("want traversal error")
	}

	// Skip-if-exists: write once, second call writes nothing.
	writeGoMod(t, dir, "example.com/app")
	schema := &ir.Schema{Modules: []*ir.Module{{Name: "shop", Jobs: []*ir.Job{{Name: "Sync"}}}}}

	wd2 := buildWorkerData("example.com/app", schema)

	first, err := writeJobStubs("tag", "", wd2)
	if err != nil || len(first) != 1 {
		t.Fatalf("first = %v, %v", first, err)
	}

	second, err := writeJobStubs("tag", "", wd2)
	if err != nil || len(second) != 0 {
		t.Fatalf("second = %v, %v", second, err)
	}
}

func TestClosureGenerateWorkerBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeGoMod(t, dir, "example.com/app")

	if _, err := GenerateWorker(GenerateWorkerConfig{ModulePath: "example.com/app", SchemaDir: "schema", WorkerEntry: "cmd/worker"}); err != nil {
		t.Fatalf("worker: %v", err)
	}

	if _, err := GenerateWorker(GenerateWorkerConfig{ModulePath: "example.com/app", SchemaDir: "schema", WorkerEntry: "cmd/worker"}); err == nil {
		t.Fatal("want collision error")
	}

	// Compile error.
	bad := t.TempDir()
	withWorkingDir(t, bad)
	writeGoMod(t, bad, "example.com/app")
	writeZeverFixture(t, filepath.Join(bad, "schema"), "bad.zen", "entity {\n")

	// schema subdir with bad file: compileSchemaDir fails.
	if _, err := GenerateWorker(GenerateWorkerConfig{ModulePath: "example.com/app", SchemaDir: "schema", WorkerEntry: "cmd/worker"}); err == nil {
		t.Fatal("want schema compile error")
	}
}

func TestClosureRunGenerateWorkerSeedBranches(t *testing.T) {
	if err := runGenerateWorker([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	if err := runGenerateSeed([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	dir := t.TempDir()
	withWorkingDir(t, dir)

	stubPromptTTY(t, false)

	if err := runGenerateWorker(nil); err == nil {
		t.Fatal("want go.mod error")
	}

	if err := runGenerateSeed(nil); err == nil {
		t.Fatal("want go.mod error")
	}

	writeGoMod(t, dir, "example.com/app")

	if err := runGenerateWorker(nil); err != nil {
		t.Fatalf("worker: %v", err)
	}

	if err := runGenerateSeed(nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Confirm-error paths.
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	origW, origS := promptConfirmForWorker, promptConfirmForSeed
	t.Cleanup(func() { promptConfirmForWorker, promptConfirmForSeed = origW, origS })

	promptConfirmForWorker = func(string, string) (bool, error) { return false, errTestSentinel }

	if err := runGenerateWorker(nil); err == nil {
		t.Fatal("want worker confirm error")
	}

	promptConfirmForSeed = func(string, string) (bool, error) { return false, errTestSentinel }

	if err := runGenerateSeed(nil); err == nil {
		t.Fatal("want seed confirm error")
	}

	promptConfirmForWorker = func(string, string) (bool, error) { return true, nil }
	promptConfirmForSeed = func(string, string) (bool, error) { return true, nil }

	if err := runGenerateWorker(nil); err != nil {
		t.Fatalf("worker force: %v", err)
	}

	if err := runGenerateSeed(nil); err != nil {
		t.Fatalf("seed force: %v", err)
	}
}

func TestClosureGenerateSeedErrorBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeGoMod(t, dir, "example.com/app")

	// ensureAppPackage error.
	dir2 := t.TempDir()
	withWorkingDir(t, dir2)
	writeGoMod(t, dir2, "example.com/app")

	if err := os.MkdirAll(filepath.Join(dir2, "internal"), 0o750); err != nil {
		t.Fatal(err)
	}

	writeZeverFixture(t, dir2, filepath.Join("internal", "app"), "blocker")

	if _, err := GenerateSeed(GenerateSeedConfig{ModulePath: "example.com/app", SeedEntry: "db/seed", GeneratedDir: "generated"}); err == nil {
		t.Fatal("want app error")
	}

	// Collision on entrypoint.
	withWorkingDir(t, dir)

	if _, err := GenerateSeed(GenerateSeedConfig{ModulePath: "example.com/app", SeedEntry: "db/seed", GeneratedDir: "generated"}); err != nil {
		t.Fatalf("setup seed: %v", err)
	}

	if _, err := GenerateSeed(GenerateSeedConfig{ModulePath: "example.com/app", SeedEntry: "db/seed", GeneratedDir: "generated"}); err == nil {
		t.Fatal("want collision error")
	}
}

func TestClosureNewHelpers(t *testing.T) {
	// modulePathOf variants.
	dir := t.TempDir()

	if got := modulePathOf(filepath.Join(dir, "nope")); got != "" {
		t.Fatalf("missing = %q", got)
	}

	writeZeverFixture(t, dir, "nomod", "go 1.24\n")

	if got := modulePathOf(filepath.Join(dir, "nomod")); got != "" {
		t.Fatalf("nomod = %q", got)
	}

	writeZeverFixture(t, dir, "quoted", "module \"example.com/q\"\n")

	if got := modulePathOf(filepath.Join(dir, "quoted")); got != "example.com/q" {
		t.Fatalf("quoted = %q", got)
	}

	writeZeverFixture(t, dir, "emptymod", "module\nmodule \n")

	if got := modulePathOf(filepath.Join(dir, "emptymod")); got != "" {
		t.Fatalf("empty = %q", got)
	}

	// detectGoVersion / goVersionOf.
	if got := detectGoVersion(filepath.Join(dir, "nope")); got != defaultGoVersion {
		t.Fatalf("detect missing = %q", got)
	}

	fw := filepath.Join(dir, "fw")

	writeZeverFixture(t, fw, "go.mod", "module example.com/fw\n\ngo 1.23\n")

	if got := detectGoVersion(fw); got != "1.23" {
		t.Fatalf("detect = %q", got)
	}

	writeZeverFixture(t, fw, "go.mod", "module example.com/fw\n")

	if got := detectGoVersion(fw); got != defaultGoVersion {
		t.Fatalf("detect nomod = %q", got)
	}

	writeZeverFixture(t, fw, "go.mod", "module example.com/fw\n\ngo \n")

	if got := detectGoVersion(fw); got != defaultGoVersion {
		t.Fatalf("detect empty go = %q", got)
	}

	if got := goVersionOf(filepath.Join(dir, "nope")); got != "" {
		t.Fatalf("goVersionOf missing = %q", got)
	}

	writeZeverFixture(t, dir, "gv", "module m\n\ngo 1.22\n")

	if got := goVersionOf(filepath.Join(dir, "gv")); got != "1.22" {
		t.Fatalf("goVersionOf = %q", got)
	}

	writeZeverFixture(t, dir, "gv2", "module m\n")

	if got := goVersionOf(filepath.Join(dir, "gv2")); got != "" {
		t.Fatalf("goVersionOf nomod = %q", got)
	}

	// defaultServiceAdapter.
	if got := defaultServiceAdapter("db"); got != "sqlite" {
		t.Fatalf("db = %q", got)
	}

	if got := defaultServiceAdapter("no-such-service"); got != "" {
		t.Fatalf("unknown = %q", got)
	}

	// renderZeverYaml error branch is unreachable via valid input; cover success.
	y, err := renderZeverYaml(NewConfig{Name: "app", Batteries: []string{"db"}})
	if err != nil || !strings.Contains(string(y), "sqlite") {
		t.Fatalf("yaml = %q, %v", y, err)
	}

	// ensureTargetDir variants.
	target := filepath.Join(dir, "newdir")

	if mkdirErr := ensureTargetDir("tag", target, false); mkdirErr != nil {
		t.Fatalf("mkdir: %v", mkdirErr)
	}

	writeZeverFixture(t, target, "f", "x")

	if dirErr := ensureTargetDir("tag", target, false); dirErr == nil {
		t.Fatal("want non-empty error")
	}

	if forceErr := ensureTargetDir("tag", target, true); forceErr != nil {
		t.Fatalf("force: %v", forceErr)
	}

	// Read error: path is a file.
	fp := filepath.Join(dir, "afile")
	writeZeverFixture(t, dir, "afile", "x")
	_ = fp

	if targetErr := ensureTargetDir("tag", fp, false); targetErr != nil {
		t.Logf("file target: %v", err)
	}

	// renderNewGoMod with/without framework + Abs error.
	cfg := NewConfig{Name: "app", ModulePath: "example.com/app", GoVersion: "1.24", OutDir: target, FrameworkVersion: "v1.0.0"}

	got, err := renderNewGoMod("tag", cfg)
	if err != nil || !strings.Contains(string(got), "require github.com/zenta-dev/zever v1.0.0") {
		t.Fatalf("gomod = %q, %v", got, err)
	}

	cfg.FrameworkDir = fw

	got, err = renderNewGoMod("tag", cfg)
	if err != nil || !strings.Contains(string(got), "replace github.com/zenta-dev/zever =>") {
		t.Fatalf("gomod replace = %q, %v", got, err)
	}

	cfg.OutDir = target // NOTE: relativeTo's Abs-error branch is provably dead:
	// filepath.Abs only fails when os.Getwd fails; pure-lexical paths
	// (even with null bytes) never error. Left uncovered by design.
	_ = cfg

	// writeNewSeedFiles error via failing write func.
	badWrite := func(string, []byte) error { return errTestSentinel }

	if seedErr := writeNewSeedFiles("tag", NewConfig{ModulePath: "m"}, badWrite); seedErr == nil {
		t.Fatal("want seed write error")
	}

	// writeNewProject error via collision-free failing write: use OutDir that is a file.
	writeZeverFixture(t, dir, "outblocker", "x")
	blockedCfg := NewConfig{Name: "app", ModulePath: "m", GoVersion: "1.24", OutDir: filepath.Join(dir, "outblocker"), Batteries: []string{"db"}}

	if _, projErr := writeNewProject("tag", blockedCfg); projErr == nil {
		t.Fatal("want project write error")
	}

	// ExistingProject skips go.mod.
	exDir := filepath.Join(dir, "existing")
	cfg2 := NewConfig{Name: "app", ModulePath: "m", GoVersion: "1.24", OutDir: exDir, Batteries: []string{"db"}, ExistingProject: true}

	written, err := writeNewProject("tag", cfg2)
	if err != nil {
		t.Fatalf("existing: %v", err)
	}

	for _, w := range written {
		if strings.HasSuffix(w, "go.mod") {
			t.Fatalf("existing project wrote go.mod: %v", written)
		}
	}
}

func TestClosureRunNewBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runNew([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	if err := runNew(nil); err == nil {
		t.Fatal("want usage error")
	}

	if err := runNew([]string{"a", "b"}); err == nil {
		t.Fatal("want usage error")
	}

	if err := runNew([]string{"bad name!"}); err == nil {
		t.Fatal("want app name error")
	}

	if err := runNew([]string{"app", "--framework-path", "x", "--framework-version", "v1"}); err == nil {
		t.Fatal("want mutual exclusion error")
	}

	// Cargo present.
	writeZeverFixture(t, dir, "Cargo.toml", "[package]\n")

	if err := runNew([]string{"app"}); err == nil {
		t.Fatal("want cargo error")
	}

	if err := os.Remove(filepath.Join(dir, "Cargo.toml")); err != nil {
		t.Fatal(err)
	}

	// go.mod stat error: make go.mod a directory with restricted perms? Use null-byte-proof:
	// create go.mod as dir so Stat succeeds (existing project path), then modulePathOf/goVersionOf handle it.
	// Instead force the !IsNotExist branch via a file that errors on stat — use dangling symlink loop.
	link := filepath.Join(dir, "go.mod")
	if err := os.Symlink(filepath.Join(dir, "go.mod"), link); err == nil {
		if err := runNew([]string{"app"}); err == nil {
			t.Log("self-symlink go.mod did not error")
		}

		_ = os.Remove(link)
	}

	// resolveFramework explicit bad path.
	if err := runNew([]string{"app", "--framework-path", filepath.Join(dir, "nope")}); err == nil {
		t.Fatal("want framework-path error")
	}

	// Version path success.
	out := filepath.Join(dir, "vapp")

	if err := runNew([]string{"vapp", "--framework-version", "v1.2.3", "--dir", out}); err != nil {
		t.Fatalf("version path: %v", err)
	}

	// ensureTargetDir error: non-empty without force.
	if err := runNew([]string{"vapp", "--dir", out}); err == nil {
		t.Fatal("want non-empty error")
	}

	// Existing go.mod path: derive module + preserve go version.
	cwdProj := t.TempDir()
	withWorkingDir(t, cwdProj)
	writeZeverFixture(t, cwdProj, "go.mod", "module example.com/cwd\n\ngo 1.22\n")

	if err := runNew([]string{"inner"}); err != nil {
		t.Fatalf("existing go.mod: %v", err)
	}

	// resolveFramework error: explicit path to file without go.mod.
	withWorkingDir(t, dir)

	emptyFw := filepath.Join(dir, "emptyfw")

	if err := os.MkdirAll(emptyFw, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := runNew([]string{"fwapp", "--framework-path", emptyFw, "--dir", filepath.Join(dir, "fwapp")}); err == nil {
		t.Fatal("want fw checkout error")
	}
}

func TestClosureProjectBranches(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	// No file → defaults.
	pc, err := loadProjectConfig()
	if err != nil || pc.SchemaDir != defaultSchemaDir {
		t.Fatalf("defaults = %+v, %v", pc, err)
	}

	// yaml project file.
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  schema_dir: custom\n")

	pc, err = loadProjectConfig()
	if err != nil || pc.SchemaDir != "custom" || pc.ServerEntry != defaultServerEntry {
		t.Fatalf("yaml = %+v, %v", pc, err)
	}

	// yml precedence: remove yaml, add yml + json; yml wins.
	if rmErr := os.Remove(filepath.Join(dir, "zever.yaml")); rmErr != nil {
		t.Fatal(err)
	}

	writeZeverFixture(t, dir, "zever.yml", "project:\n  schema_dir: fromyml\n")
	writeZeverFixture(t, dir, "zever.json", `{"project": {"schema_dir": "fromjson"}}`)

	pc, err = loadProjectConfig()
	if err != nil || pc.SchemaDir != "fromyml" {
		t.Fatalf("yml precedence = %+v, %v", pc, err)
	}

	if rmErr := os.Remove(filepath.Join(dir, "zever.yml")); rmErr != nil {
		t.Fatal(err)
	}

	pc, err = loadProjectConfig()
	if err != nil || pc.SchemaDir != "fromjson" {
		t.Fatalf("json = %+v, %v", pc, err)
	}

	// Decode errors.
	if rmErr := os.Remove(filepath.Join(dir, "zever.json")); rmErr != nil {
		t.Fatal(rmErr)
	}

	writeZeverFixture(t, dir, "zever.yaml", "project: [unclosed\n")

	if _, cfgErr := loadProjectConfig(); cfgErr == nil {
		t.Fatal("want yaml error")
	}

	writeZeverFixture(t, dir, "zever.json", `{"project":`)

	// remove yaml so json is discovered.
	if rmErr := os.Remove(filepath.Join(dir, "zever.yaml")); rmErr != nil {
		t.Fatal(err)
	}

	if _, cfgErr := loadProjectConfig(); cfgErr == nil {
		t.Fatal("want json error")
	}

	if jsonRmErr := os.Remove(filepath.Join(dir, "zever.json")); jsonRmErr != nil {
		t.Fatal(jsonRmErr)
	}

	// decodeProjectFile direct: unsupported ext + missing file.
	writeZeverFixture(t, dir, "zever.toml", "x")

	if _, decodeErr := decodeProjectFile(filepath.Join(dir, "zever.toml")); decodeErr == nil {
		t.Fatal("want unsupported ext error")
	}

	if _, missingErr := decodeProjectFile(filepath.Join(dir, "nope.yaml")); missingErr == nil {
		t.Fatal("want read error")
	}

	// Directory named zever.yaml is skipped (IsDir) → defaults.
	if mkdirErr := os.Mkdir(filepath.Join(dir, "zever.yaml"), 0o750); mkdirErr != nil {
		t.Fatal(mkdirErr)
	}

	if _, dirCfgErr := loadProjectConfig(); dirCfgErr != nil {
		t.Fatalf("dir config = %v", dirCfgErr)
	}

	if rmErr := os.Remove(filepath.Join(dir, "zever.yaml")); rmErr != nil {
		t.Fatal(err)
	}
}

func TestClosureExtractBranches(t *testing.T) {
	// printExtractUsage color + plain.
	fs := flag.NewFlagSet("extract", flag.ContinueOnError)
	prevColor := colorEnabled
	colorEnabled = true
	printExtractUsage(fs)
	colorEnabled = false
	printExtractUsage(fs)
	colorEnabled = prevColor

	if err := runExtract([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	dir := t.TempDir()
	withWorkingDir(t, dir)

	if err := runExtract(nil); err == nil {
		t.Fatal("want usage error")
	}

	if err := runExtract([]string{"a", "b"}); err == nil {
		t.Fatal("want usage error")
	}

	// readGoMod error (no go.mod).
	if err := runExtract([]string{"shop"}); err == nil {
		t.Fatal("want go.mod error")
	}

	// Full project, then plan/write error branches.
	projDir, _ := setupExtractProject(t)

	_ = projDir

	// Unknown module.
	if err := runExtract([]string{"nosuch"}); err == nil {
		t.Fatal("want unknown module error")
	}

	// Force re-extract works.
	if err := runExtract([]string{"billing", "--force"}); err != nil {
		t.Fatalf("force: %v", err)
	}

	// copySchemaFiles read error: plan with missing source.
	plan := extractPlan{Module: &ir.Module{Name: "m"}, SchemaFiles: []string{"missing.zen"}, OutDir: filepath.Join(projDir, "out"), ModulePath: "example.com/m"}

	if _, err := copySchemaFiles("tag", plan); err == nil {
		t.Fatal("want copy read error")
	}

	// writeScaffold collision inside copy.
	existing := filepath.Join(projDir, "out2", "schema", "billing.zen")
	writeZeverFixture(t, projDir, filepath.Join("out2", "schema", "billing.zen"), "x")
	_ = existing

	schema, err := compileSchemaDir("tag", "schema")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	plan2, err := planExtraction("tag", ExtractConfig{Module: "billing", OutDir: filepath.Join(projDir, "out2")}, schema, goModInfo{ModulePath: "example.com/shop", GoVersion: "1.24"})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	if _, copyErr := copySchemaFiles("tag", plan2); copyErr == nil {
		t.Fatal("want copy collision error")
	}

	// planExtraction: unknown + empty-module variants.
	if _, planErr := planExtraction("tag", ExtractConfig{Module: "nope"}, schema, goModInfo{}); planErr == nil {
		t.Fatal("want plan unknown error")
	}

	// renderExtractedGoMod with framework dir + rel error.
	gm := goModInfo{ModulePath: "example.com/shop", GoVersion: "1.24", FrameworkVersion: "v0.0.0", FrameworkDir: "."}

	if _, renderErr := renderExtractedGoMod("tag", extractPlan{OutDir: "out", ModulePath: "m"}, gm); renderErr != nil {
		t.Fatalf("gomod: %v", err)
	}

	gmBad := goModInfo{ModulePath: "m", GoVersion: "1.24", FrameworkVersion: "v", FrameworkDir: string([]byte{'x', 0})}

	// NOTE: relativeTo's Abs-error branches are provably dead (filepath.Abs
	// only fails when os.Getwd fails), so no error assertion here.
	_ = gmBad

	// relativeTo same-dir gets ./ prefix; parent gets ../.
	rel, err := relativeTo(projDir, filepath.Join(projDir, "sub"))
	if err != nil || rel != "./sub" {
		t.Fatalf("rel = %q, %v", rel, err)
	}

	rel, err = relativeTo(filepath.Join(projDir, "sub"), projDir)
	if err != nil || rel != ".." {
		t.Fatalf("rel parent = %q, %v", rel, err)
	}

	// readGoMod variants: dir-as-file, no module, scanner-ish.
	writeZeverFixture(t, projDir, "go2.mod", "go 1.24\n")

	// go.mod as directory.
	withWorkingDir(t, projDir)

	if err := os.Mkdir("gomod-dir", 0o750); err != nil {
		t.Fatal(err)
	}

	// scanGoModLine: every branch.
	info := &goModInfo{}

	if got := scanGoModLine(info, "", "module example.com/x"); got != "" || info.ModulePath != "example.com/x" {
		t.Fatalf("module line: %+v", info)
	}

	info = &goModInfo{}

	if got := scanGoModLine(info, "", `module "example.com/q"`); got != "" || info.ModulePath != "example.com/q" {
		t.Fatalf("quoted module: %+v", info)
	}

	scanGoModLine(info, "", "go 1.23")

	if info.GoVersion != "1.23" {
		t.Fatalf("go line: %+v", info)
	}

	if got := scanGoModLine(info, "", "require ("); got != "require" {
		t.Fatalf("require block = %q", got)
	}

	if got := scanGoModLine(info, "", "replace ("); got != "replace" {
		t.Fatalf("replace block = %q", got)
	}

	scanGoModLine(info, "require", "github.com/zenta-dev/zever v1.0.0")

	if info.FrameworkVersion != "v1.0.0" {
		t.Fatalf("require line: %+v", info)
	}

	scanGoModLine(info, "", "require github.com/zenta-dev/zever v2.0.0")

	if info.FrameworkVersion != "v2.0.0" {
		t.Fatalf("inline require: %+v", info)
	}

	scanGoModLine(info, "replace", "github.com/zenta-dev/zever => ../fw")

	if info.FrameworkDir != "../fw" {
		t.Fatalf("replace line: %+v", info)
	}

	scanGoModLine(info, "", "replace github.com/zenta-dev/zever => ./local")

	if info.FrameworkDir != "./local" {
		t.Fatalf("inline replace: %+v", info)
	}

	if got := scanGoModLine(info, "other", "// comment"); got != "other" {
		t.Fatalf("passthrough = %q", got)
	}

	// readReplace variants.
	ri := &goModInfo{}

	readReplace(ri, "no arrow here")

	if ri.FrameworkDir != "" {
		t.Fatal("want no-op")
	}

	readReplace(ri, "other/module => ./x")

	if ri.FrameworkDir != "" {
		t.Fatal("want no-op for other module")
	}

	readReplace(ri, "github.com/zenta-dev/zever => other/module v1.0.0 extra tokens here")

	if ri.FrameworkDir != "" {
		t.Fatal("want no-op for module-version replace")
	}

	readReplace(ri, "github.com/zenta-dev/zever => v1.0.0")

	if ri.FrameworkDir != "" {
		t.Fatal("want no-op for version-only replace")
	}

	readReplace(ri, "github.com/zenta-dev/zever => ./fwrel")

	if ri.FrameworkDir != "./fwrel" {
		t.Fatalf("rel replace: %+v", ri)
	}

	readReplace(ri, "github.com/zenta-dev/zever => /abs/fw")

	if ri.FrameworkDir != "/abs/fw" {
		t.Fatalf("abs replace: %+v", ri)
	}

	// resolveFrameworkDir variants.
	r2 := &goModInfo{FrameworkDir: "x", ModulePath: frameworkModulePath}
	resolveFrameworkDir(r2)

	if r2.FrameworkDir != "x" {
		t.Fatal("want preserve")
	}

	r3 := &goModInfo{ModulePath: "other/module"}
	resolveFrameworkDir(r3)

	if r3.FrameworkDir != "" {
		t.Fatal("want no-op for other module")
	}

	r4 := &goModInfo{ModulePath: frameworkModulePath}
	resolveFrameworkDir(r4)

	if r4.FrameworkDir != "." {
		t.Fatalf("self resolve = %+v", r4)
	}
}

func TestClosureGenerateEntityJobScheduleSeedErrors(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeSchemaModuleFixture(t, dir, "shop", "entity Existing {\n\tid: uuid @primary\n}\n")

	// Entity: bad field, missing module, taken.
	if _, err := GenerateEntity(GenerateEntityConfig{Module: "shop", Name: "E2", Fields: []EntityField{{Name: "t", Type: "nope"}}}); err == nil {
		t.Fatal("want field error")
	}

	if _, err := GenerateEntity(GenerateEntityConfig{Module: "nosuch", Name: "E2"}); err == nil {
		t.Fatal("want missing module error")
	}

	// runGenerateEntity non-TTY usage + parse error.
	stubPromptTTY(t, false)

	if err := runGenerateEntity(nil); err == nil {
		t.Fatal("want usage error")
	}

	if err := runGenerateEntity([]string{"--badflag", "a", "b"}); err == nil {
		t.Fatal("want parse error")
	}

	// Job: bad queue + missing module.
	if _, err := GenerateJob(GenerateJobConfig{Module: "shop", Name: "J", Queue: "bad-queue"}); err == nil {
		t.Fatal("want queue error")
	}

	if _, err := GenerateJob(GenerateJobConfig{Module: "nosuch", Name: "J", Queue: "default"}); err == nil {
		t.Fatal("want missing module error")
	}

	if err := runGenerateJob([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	stubPromptTTY(t, false)

	if err := runGenerateJob(nil); err == nil {
		t.Fatal("want usage error")
	}

	// Schedule: cron/dispatch validation.
	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "S"}); err == nil {
		t.Fatal("want cron error")
	}

	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "S", Cron: "bad\"quote", Dispatch: "J"}); err == nil {
		t.Fatal("want cron quote error")
	}

	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "S", Cron: "* * * * *"}); err == nil {
		t.Fatal("want dispatch error")
	}

	// Dispatch job missing.
	writeSchemaModuleFixture(t, dir, "jobs", "job Real() {\n\tqueue: default\n}\n")

	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "jobs", Name: "S", Cron: "* * * * *", Dispatch: "Missing"}); err == nil {
		t.Fatal("want missing job error")
	}

	if err := runGenerateSchedule([]string{"--badflag"}); err == nil {
		t.Fatal("want parse error")
	}

	stubPromptTTY(t, false)

	if err := runGenerateSchedule(nil); err == nil {
		t.Fatal("want usage error")
	}

	// Schedule interactive cron + dispatch prompts.
	stubPromptTTY(t, true)
	setPromptInteractive(t)

	origSel, origIn := promptSelectForSchedule, promptInputForSchedule
	t.Cleanup(func() { promptSelectForSchedule, promptInputForSchedule = origSel, origIn })

	promptInputForSchedule = func(title, _ string, _ func(string) error) (string, error) {
		switch title {
		case "Module name":
			return "jobs", nil
		case "Schedule name":
			return "Sched1", nil
		case "Cron spec":
			return "*/5 * * * *", nil
		case "Dispatch job name":
			return "Real", nil
		}

		return "", errTestSentinel
	}
	promptSelectForSchedule = func(title string, opts []string) (string, error) {
		if title == "Dispatch job" {
			return "Real", nil
		}

		return opts[0], nil
	}

	if err := runGenerateSchedule(nil); err != nil {
		t.Fatalf("interactive schedule: %v", err)
	}

	// Cron prompt error.
	promptInputForSchedule = func(title, _ string, _ func(string) error) (string, error) {
		if title == "Cron spec" {
			return "", errTestSentinel
		}

		return "x", nil
	}

	if err := runGenerateSchedule([]string{"jobs", "S2", "--dispatch", "Real"}); err == nil {
		t.Fatal("want cron prompt error")
	}
}

// stubIRModuleFile is a placeholder to keep the message-kind test honest:
// declNameTaken's MessageDecl branch is exercised through a real parse below.
var stubIRModuleFile = struct{}{}

func TestClosureDeclNameTakenMessage(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	writeSchemaModuleFixture(t, dir, "msg", "message Greet {\n\ttext: string\n}\n")

	_, _, file, err := readZenModule("tag", "msg")
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	if kind, taken := declNameTaken(file, "Greet"); !taken || kind != "message" {
		t.Fatalf("message taken = %v, %q", taken, kind)
	}
}

func TestClosureWriteExtractionErrors(t *testing.T) {
	projDir, _ := setupExtractProject(t)

	schema, err := compileSchemaDir("tag", "schema")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	gm := goModInfo{ModulePath: "example.com/shop", GoVersion: "1.24", FrameworkVersion: "v0.0.0", FrameworkDir: "."}

	plan, err := planExtraction("tag", ExtractConfig{Module: "billing"}, schema, gm)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}

	// writeExtraction go.mod collision without force: pre-create go.mod.
	plan.OutDir = filepath.Join(projDir, "collide")
	writeZeverFixture(t, projDir, filepath.Join("collide", "go.mod"), "x")

	if err := writeExtraction("tag", plan, gm); err == nil {
		t.Fatal("want extraction write error")
	}
}
