package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildZeverBin builds the CLI once into a temp dir for exit-code tests.
func buildZeverBin(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "zever")

	out, err := exec.CommandContext(context.Background(), "go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	return bin
}

func TestBinaryUnknownSubcommandErrorFirst(t *testing.T) {
	bin := buildZeverBin(t)

	cmd := exec.CommandContext(context.Background(), bin, "comple")

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	err := cmd.Run()

	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("exit = %v, want 1", err)
	}

	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) > 3 {
		t.Fatalf("stderr should be short, got %d lines:\n%s", len(lines), stderr.String())
	}

	if !strings.Contains(lines[0], `unknown subcommand "comple"`) {
		t.Fatalf("first line = %q", lines[0])
	}

	if !strings.Contains(stderr.String(), `did you mean "compile"?`) {
		t.Fatalf("missing hint:\n%s", stderr.String())
	}
}

func TestBinaryDBMigrateHelpExitsZeroWithoutNoise(t *testing.T) {
	bin := buildZeverBin(t)

	cmd := exec.CommandContext(context.Background(), bin, "db", "migrate", "--help")

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("db migrate --help: %v\nstderr:\n%s", err, stderr.String())
	}

	if strings.Contains(stderr.String(), "help requested") || strings.Contains(stdout.String(), "help requested") {
		t.Fatalf("help noise present:\n%s", stderr.String())
	}
}

func TestEnforceMigrateLintErrorsGoToStderrAsError(t *testing.T) {
	var err error

	var stderr string

	stdout := captureZeverStdout(t, func() {
		stderr = captureZeverStderr(t, func() {
			err = enforceMigrateLint("postgres", []string{`ALTER TABLE "public"."users" DROP COLUMN IF EXISTS "email";`})
		})
	})

	if err == nil {
		t.Fatal("expected lint failure")
	}

	if !strings.Contains(stderr, "error:") || !strings.Contains(stderr, "drop_column") {
		t.Fatalf("stderr = %q, want error: line naming drop_column", stderr)
	}

	if strings.Contains(stdout, "warning") || strings.Contains(stdout, "drop_column") {
		t.Fatalf("error finding must not be a stdout warning, got %q", stdout)
	}
}

func TestMigrateLintDryRunPostgresWithoutDSNIsOffline(t *testing.T) {
	dir := t.TempDir()
	schema := writeZeverFixture(t, dir, "schema.zen", lintDropSchema)

	var err error

	_ = captureZeverStdout(t, func() {
		err = runDBMigrate([]string{"--lint", "--dry-run", "--adapter=postgres", schema})
	})

	if err != nil {
		t.Fatalf("offline lint dry-run must not connect or fail: %v", err)
	}
}
