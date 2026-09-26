package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/breaking"
)

// inspectBreakingOldSchema / New remove one field: a breaking change.
const inspectBreakingOldSchema = `entity User {
	id: uuid @primary
	email: string @unique
	name: string
}
`

const inspectBreakingNewSchema = `entity User {
	id: uuid @primary
	email: string @unique
}
`

// inspectBreakingAdditiveSchema only adds an entity: informational, not breaking.
const inspectBreakingAdditiveSchema = `entity User {
	id: uuid @primary
	email: string @unique
	name: string
}

entity Team {
	id: uuid @primary
}
`

func TestRunBreakingNoSeparator(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	err := runBreaking([]string{schemaPath})
	if !errors.Is(err, errNoSeparator) {
		t.Fatalf("error = %v, want errNoSeparator", err)
	}
}

func TestRunBreakingHelpPrintsUsage(t *testing.T) {
	if err := runBreaking([]string{"--help"}); err != nil {
		t.Fatalf("runBreaking --help: %v", err)
	}
}

func TestRunBreakingNoDifferences(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old/user.zen", inspectUserSchema)
	newPath := writeInspectFixture(t, dir, "new/user.zen", inspectUserSchema)

	var out bytes.Buffer
	err := runBreakingWith(BreakingConfig{OldFiles: []string{oldPath}, NewFiles: []string{newPath}, Out: &out})
	if err != nil {
		t.Fatalf("runBreakingWith identical schemas: %v", err)
	}

	if !strings.Contains(out.String(), "no differences found") {
		t.Fatalf("output = %q, want no-differences message", out.String())
	}
}

// TestRunBreakingExpandsBareDirectories proves each side of `--` accepts a
// bare directory instead of requiring an explicit glob/file list, and that
// discovery recurses into arbitrarily nested subdirectories (a versioned
// schema/v1/*.zen-style layout) exactly like every other command's
// auto-discovery.
func TestRunBreakingExpandsBareDirectories(t *testing.T) {
	dir := t.TempDir()
	oldDir := filepath.Join(dir, "schema-old")
	newDir := filepath.Join(dir, "schema-new")
	writeInspectFixture(t, oldDir, "v1/user.zen", inspectBreakingOldSchema)
	writeInspectFixture(t, newDir, "v1/user.zen", inspectBreakingNewSchema)

	var out bytes.Buffer
	err := runBreakingWith(BreakingConfig{OldFiles: []string{oldDir}, NewFiles: []string{newDir}, Out: &out})
	if err == nil {
		t.Fatalf("expected error for breaking change discovered via bare directories")
	}

	if !strings.Contains(err.Error(), "breaking change") {
		t.Fatalf("error = %v, want a breaking-change error", err)
	}
}

func TestRunBreakingReportsBreakingChange(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old/user.zen", inspectBreakingOldSchema)
	newPath := writeInspectFixture(t, dir, "new/user.zen", inspectBreakingNewSchema)

	var out bytes.Buffer
	err := runBreakingWith(BreakingConfig{OldFiles: []string{oldPath}, NewFiles: []string{newPath}, Out: &out})
	if err == nil {
		t.Fatalf("expected error for breaking change, got nil")
	}

	if !strings.Contains(err.Error(), "breaking change(s) found") {
		t.Fatalf("error = %q, want breaking-change count", err.Error())
	}

	if out.String() == "" {
		t.Fatalf("expected breaking changes printed to out")
	}
}

func TestRunBreakingInformationalOnlySucceeds(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old/user.zen", inspectBreakingOldSchema)
	newPath := writeInspectFixture(t, dir, "new/user.zen", inspectBreakingAdditiveSchema)

	var out bytes.Buffer
	err := runBreakingWith(BreakingConfig{OldFiles: []string{oldPath}, NewFiles: []string{newPath}, Out: &out})
	if err != nil {
		t.Fatalf("runBreakingWith additive change: %v", err)
	}

	if !strings.Contains(out.String(), "no breaking changes") {
		t.Fatalf("output = %q, want no-breaking-changes message", out.String())
	}
}

func TestRunBreakingViaShellFindsBreaking(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old/user.zen", inspectBreakingOldSchema)
	newPath := writeInspectFixture(t, dir, "new/user.zen", inspectBreakingNewSchema)

	if err := runBreaking([]string{oldPath, "--", newPath}); err == nil {
		t.Fatalf("expected error for breaking change, got nil")
	}
}

func TestRunBreakingEmptyOldFiles(t *testing.T) {
	dir := t.TempDir()
	newPath := writeInspectFixture(t, dir, "new/user.zen", inspectUserSchema)

	var out bytes.Buffer
	if err := runBreakingWith(BreakingConfig{NewFiles: []string{newPath}, Out: &out}); err == nil {
		t.Fatalf("expected error for empty old file list, got nil")
	}
}

func TestRunBreakingEmptyNewFiles(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old/user.zen", inspectUserSchema)

	var out bytes.Buffer
	err := runBreakingWith(BreakingConfig{OldFiles: []string{oldPath}, Out: &out})
	if err == nil {
		t.Fatalf("expected error for empty new file list, got nil")
	}

	if !strings.Contains(err.Error(), "new schema") {
		t.Fatalf("error = %q, want it attributed to the new schema", err.Error())
	}
}

func TestRunBreakingOldCompileError(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old/broken.zen", inspectBrokenSchema)
	newPath := writeInspectFixture(t, dir, "new/user.zen", inspectUserSchema)

	var out bytes.Buffer
	err := runBreakingWith(BreakingConfig{OldFiles: []string{oldPath}, NewFiles: []string{newPath}, Out: &out})
	if err == nil {
		t.Fatalf("expected error for broken old schema, got nil")
	}

	if !strings.Contains(err.Error(), "old schema") {
		t.Fatalf("error = %q, want it attributed to the old schema", err.Error())
	}
}

func TestRunBreakingNewCompileError(t *testing.T) {
	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old/user.zen", inspectUserSchema)
	newPath := writeInspectFixture(t, dir, "new/broken.zen", inspectBrokenSchema)

	var out bytes.Buffer
	err := runBreakingWith(BreakingConfig{OldFiles: []string{oldPath}, NewFiles: []string{newPath}, Out: &out})
	if err == nil {
		t.Fatalf("expected error for broken new schema, got nil")
	}

	if !strings.Contains(err.Error(), "new schema") {
		t.Fatalf("error = %q, want it attributed to the new schema", err.Error())
	}
}

func TestCountBreaking(t *testing.T) {
	changes := []breaking.Change{
		{Kind: breaking.KindEntityRemoved, Message: "removed entity X", Breaking: true},
		{Kind: breaking.KindEntityAdded, Message: "added entity Y", Breaking: false},
		{Kind: breaking.KindFieldRemoved, Message: "removed field Z", Breaking: true},
	}

	if got := countBreaking(changes); got != 2 {
		t.Fatalf("countBreaking = %d, want 2", got)
	}

	if got := countBreaking(nil); got != 0 {
		t.Fatalf("countBreaking(nil) = %d, want 0", got)
	}
}
