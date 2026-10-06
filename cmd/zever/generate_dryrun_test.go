package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateModuleDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	var buf bytes.Buffer

	path, err := GenerateModule(GenerateModuleConfig{Name: "shop", SchemaDir: "schema", Stdout: &buf, DryRun: true})
	if err != nil {
		t.Fatalf("GenerateModule dry-run: %v", err)
	}

	if path == "" {
		t.Fatal("dry run returned empty path")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "schema")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created the schema dir")
	}

	if !strings.Contains(buf.String(), "dry run") {
		t.Fatalf("output missing dry-run marker:\n%s", buf.String())
	}
}

func TestRunGenerateModuleDryRunFlag(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	out := captureStdout(t, func() {
		if err := runGenerateModule([]string{"module", "shop", "--dry-run"}); err != nil {
			t.Fatalf("runGenerateModule --dry-run: %v", err)
		}
	})

	if _, statErr := os.Stat(filepath.Join(dir, "schema")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created the schema dir")
	}

	if !strings.Contains(out, "dry run") {
		t.Fatalf("output missing dry-run marker:\n%s", out)
	}
}

func TestGenerateEntityDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")
	before := readFile(t, path)

	var buf bytes.Buffer

	got, err := GenerateEntity(GenerateEntityConfig{
		Module: "shop",
		Name:   "Order",
		Fields: []EntityField{{Name: "title", Type: "string"}},
		Stdout: &buf,
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("GenerateEntity dry-run: %v", err)
	}

	if !strings.HasSuffix(path, got) {
		t.Fatalf("path = %q, want suffix %q", path, got)
	}

	if after := readFile(t, path); after != before {
		t.Fatal("dry run modified the schema file")
	}

	if out := buf.String(); !strings.Contains(out, "would append") || !strings.Contains(out, "entity Order") {
		t.Fatalf("output missing preview:\n%s", out)
	}
}

func TestRunGenerateEntityDryRunFlag(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")
	before := readFile(t, path)

	out := captureStdout(t, func() {
		if err := runGenerateEntity([]string{"shop", "Order", "--field", "title:string", "--dry-run"}); err != nil {
			t.Fatalf("runGenerateEntity --dry-run: %v", err)
		}
	})

	if after := readFile(t, path); after != before {
		t.Fatal("dry run modified the schema file")
	}

	if !strings.Contains(out, "would append") {
		t.Fatalf("output missing preview:\n%s", out)
	}
}

func TestGenerateJobDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "")
	before := readFile(t, path)

	var buf bytes.Buffer

	if _, err := GenerateJob(GenerateJobConfig{Module: "shop", Name: "Ship", Queue: "default", Stdout: &buf, DryRun: true}); err != nil {
		t.Fatalf("GenerateJob dry-run: %v", err)
	}

	if after := readFile(t, path); after != before {
		t.Fatal("dry run modified the schema file")
	}

	if !strings.Contains(buf.String(), "would append") {
		t.Fatalf("output missing preview:\n%s", buf.String())
	}
}

func TestGenerateScheduleDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	path := writeModuleFixture(t, dir, "shop", "job Ship() {\n\tqueue: default\n}\n")
	before := readFile(t, path)

	var buf bytes.Buffer

	if _, err := GenerateSchedule(GenerateScheduleConfig{Module: "shop", Name: "Nightly", Cron: "0 0 * * *", Dispatch: "Ship", Stdout: &buf, DryRun: true}); err != nil {
		t.Fatalf("GenerateSchedule dry-run: %v", err)
	}

	if after := readFile(t, path); after != before {
		t.Fatal("dry run modified the schema file")
	}

	if !strings.Contains(buf.String(), "would append") {
		t.Fatalf("output missing preview:\n%s", buf.String())
	}
}

func TestGenerateServerDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	var buf bytes.Buffer

	res, err := GenerateServer(GenerateServerConfig{
		ModulePath:  "example.com/shop",
		SchemaDir:   "schema",
		ServerEntry: "cmd/server",
		Stdout:      &buf,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("GenerateServer dry-run: %v", err)
	}

	if res.Entrypoint == "" {
		t.Fatal("dry run returned empty entrypoint")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "cmd")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created cmd/")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "internal")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created internal/")
	}

	if !strings.Contains(buf.String(), "dry run") {
		t.Fatalf("output missing dry-run marker:\n%s", buf.String())
	}
}

func TestGenerateWorkerDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	var buf bytes.Buffer

	res, err := GenerateWorker(GenerateWorkerConfig{
		ModulePath:  "example.com/shop",
		SchemaDir:   "schema",
		WorkerEntry: "cmd/worker",
		Stdout:      &buf,
		DryRun:      true,
	})
	if err != nil {
		t.Fatalf("GenerateWorker dry-run: %v", err)
	}

	if res.Entrypoint == "" {
		t.Fatal("dry run returned empty entrypoint")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "cmd")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created cmd/")
	}

	if !strings.Contains(buf.String(), "dry run") {
		t.Fatalf("output missing dry-run marker:\n%s", buf.String())
	}
}

func TestGenerateSeedDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)
	writeGoMod(t, dir, "example.com/shop")

	var buf bytes.Buffer

	res, err := GenerateSeed(GenerateSeedConfig{
		ModulePath: "example.com/shop",
		SeedEntry:  "db/seed",
		Stdout:     &buf,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("GenerateSeed dry-run: %v", err)
	}

	if res.Entrypoint == "" {
		t.Fatal("dry run returned empty entrypoint")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "db")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created db/")
	}

	if !strings.Contains(buf.String(), "dry run") {
		t.Fatalf("output missing dry-run marker:\n%s", buf.String())
	}
}

func TestGenerateTinkerDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	var buf bytes.Buffer

	got, err := GenerateTinker(GenerateTinkerConfig{
		AppPackage: "example.com/shop/internal/app",
		OutDir:     "cmd/tinker-shim",
		Stdout:     &buf,
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("GenerateTinker dry-run: %v", err)
	}

	if got == "" {
		t.Fatal("dry run returned empty path")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "cmd")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created cmd/")
	}

	if !strings.Contains(buf.String(), "dry run") {
		t.Fatalf("output missing dry-run marker:\n%s", buf.String())
	}
}

func TestGenerateAdapterDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	withWorkingDir(t, dir)

	var buf bytes.Buffer

	res, err := GenerateAdapter(GenerateAdapterConfig{
		Battery: "cache",
		Name:    "example",
		Stdout:  &buf,
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("GenerateAdapter dry-run: %v", err)
	}

	if res.AdapterPath == "" {
		t.Fatal("dry run returned empty adapter path")
	}

	if _, statErr := os.Stat(filepath.Join(dir, "adapters")); !os.IsNotExist(statErr) {
		t.Fatal("dry run created adapters/")
	}

	if out := buf.String(); !strings.Contains(out, "dry run") || !strings.Contains(out, res.AdapterPath) {
		t.Fatalf("output missing preview:\n%s", out)
	}
}
