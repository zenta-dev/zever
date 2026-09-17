package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRoutesNoFiles(t *testing.T) {
	err := runRoutes(nil)
	if !errors.Is(err, errNoInputFiles) {
		t.Fatalf("error = %v, want errNoInputFiles", err)
	}
}

func TestRunRoutesPrintsHTTPBindings(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runRoutes([]string{schemaPath})
	})

	if runErr != nil {
		t.Fatalf("runRoutes: %v", runErr)
	}

	const want = `GET /v1/users/{id} -> (root).UserService.GetUser`
	if !strings.Contains(output, want) {
		t.Fatalf("output = %q, want it to contain %q", output, want)
	}
}

func TestRunRoutesCompileError(t *testing.T) {
	dir := t.TempDir()
	schemaPath := filepath.Join(dir, "broken.zen")
	writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)

	if err := runRoutes([]string{schemaPath}); err == nil {
		t.Fatalf("expected error for broken schema, got nil")
	}
}

func TestRunRoutesWithCoreSkipsNonHTTP(t *testing.T) {
	dir := t.TempDir()
	schema := `entity User {
	id: uuid @primary
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		auth: required
	}
}
`
	schemaPath := writeInspectFixture(t, dir, "user.zen", schema)

	var out bytes.Buffer
	if err := runRoutesWith(RoutesConfig{Files: []string{schemaPath}, Out: &out}); err != nil {
		t.Fatalf("runRoutesWith: %v", err)
	}

	if out.String() != "" {
		t.Fatalf("output = %q, want nothing for RPCs without HTTP bindings", out.String())
	}
}

func TestRunRoutesWithCoreColoredMethods(t *testing.T) {
	dir := t.TempDir()
	schema := `entity User {
	id: uuid @primary
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		http: GET "/v1/users/{id}"
		auth: required
	}
	rpc CreateUser(email: string) -> User {
		http: POST "/v1/users"
		auth: required
	}
	rpc DeleteUser(id: uuid) -> User {
		http: DELETE "/v1/users/{id}"
		auth: required
	}
	rpc ReplaceUser(id: uuid) -> User {
		http: PUT "/v1/users/{id}"
		auth: required
	}
}
`
	schemaPath := writeInspectFixture(t, dir, "user.zen", schema)

	prev := colorEnabled
	colorEnabled = true
	defer func() { colorEnabled = prev }()

	var out bytes.Buffer
	if err := runRoutesWith(RoutesConfig{Files: []string{schemaPath}, Out: &out}); err != nil {
		t.Fatalf("runRoutesWith colored: %v", err)
	}

	got := out.String()
	for _, want := range []string{"GET", "POST", "DELETE", "PUT", "/v1/users", "UserService"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output = %q, want it to contain %q", got, want)
		}
	}
}

func TestRunRoutesWithCoreEmptyFiles(t *testing.T) {
	var out bytes.Buffer
	if err := runRoutesWith(RoutesConfig{Out: &out}); err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
}

func TestRunRoutesAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/app.zen", inspectUserSchema)
	t.Chdir(dir)
	t.Setenv("ZEVER_NO_HINT", "1")

	var runErr error
	output := inspectCaptureOutput(t, func() {
		runErr = runRoutes(nil)
	})
	if runErr != nil {
		t.Fatalf("runRoutes auto-discovered: %v (output = %q)", runErr, output)
	}
	if !strings.Contains(output, "GET /v1/users/{id}") {
		t.Fatalf("output = %q, want discovered route", output)
	}
}
