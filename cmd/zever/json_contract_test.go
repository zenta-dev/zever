package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
)

// mustJSONMode enables jsonMode with jsonOut redirected to buf for the
// duration of the test. Callers must stay sequential (no t.Parallel):
// jsonOut is a package-global sink.
func mustJSONMode(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer

	oldOut, oldMode, oldExplicit := jsonOut, jsonMode, jsonExplicit
	jsonOut, jsonMode, jsonExplicit = &buf, true, true
	t.Cleanup(func() { jsonOut, jsonMode, jsonExplicit = oldOut, oldMode, oldExplicit })
	t.Setenv("ZEVER_NO_HINT", "1")

	return &buf
}

// mustDecodeEnvelope decodes the single JSON envelope written to buf.
func mustDecodeEnvelope(t *testing.T, buf *bytes.Buffer) Envelope {
	t.Helper()

	var env Envelope
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &env); err != nil {
		t.Fatalf("envelope is not JSON: %v (raw %q)", err, buf.String())
	}

	return env
}

// envelopeData re-decodes the envelope's Data payload into a plain map.
func envelopeData(t *testing.T, env Envelope) map[string]any {
	t.Helper()

	raw, err := json.Marshal(env.Data)
	if err != nil {
		t.Fatalf("marshal envelope data: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("envelope data is not an object: %v (raw %s)", err, raw)
	}

	return data
}

func TestRoutesJSONEmitsEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	stdout := inspectCaptureOutput(t, func() {
		if err := runRoutes([]string{schemaPath}); err != nil {
			t.Fatalf("runRoutes --json: %v", err)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q in JSON mode, want only the envelope on the JSON sink", stdout)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "routes" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok routes envelope", env)
	}

	data := envelopeData(t, env)
	routes, ok := data["routes"].([]any)
	if !ok || len(routes) != 1 {
		t.Fatalf("data.routes = %v, want one route", data["routes"])
	}

	route, ok := routes[0].(map[string]any)
	if !ok || route["method"] != "GET" || route["path"] != "/v1/users/{id}" {
		t.Fatalf("route = %v, want GET /v1/users/{id}", routes[0])
	}
}

func TestConfigShowJSONEmitsEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	stdout := inspectCaptureOutput(t, func() {
		if err := runConfigShow(nil); err != nil {
			t.Fatalf("runConfigShow --json: %v", err)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q in JSON mode, want only the envelope on the JSON sink", stdout)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "config show" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok config show envelope", env)
	}

	data := envelopeData(t, env)
	services, ok := data["services"].(map[string]any)
	if !ok || len(services) == 0 {
		t.Fatalf("data.services = %v, want redacted service map", data["services"])
	}

	db, ok := services["db"].(map[string]any)
	if !ok || db["options"] == nil {
		t.Fatalf("data.services.db = %v, want options entry", services["db"])
	}
	if _, bad := db["adapter"]; bad {
		t.Fatalf("data.services.db = %v, want no adapter entry (drivers are code-wired)", services["db"])
	}
}

func TestGraphJSONEmitsEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	stdout := inspectCaptureOutput(t, func() {
		if err := runGraph([]string{schemaPath}); err != nil {
			t.Fatalf("runGraph --json: %v", err)
		}
	})

	if stdout != "" {
		t.Fatalf("stdout = %q in JSON mode, want only the envelope on the JSON sink", stdout)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "graph" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok graph envelope", env)
	}

	data := envelopeData(t, env)
	mermaid, ok := data["mermaid"].(string)
	if !ok || !strings.Contains(mermaid, "graph TD") {
		t.Fatalf("data.mermaid = %v, want Mermaid diagrams", data["mermaid"])
	}

	modules, ok := data["modules"].([]any)
	if !ok || len(modules) == 0 {
		t.Fatalf("data.modules = %v, want module list", data["modules"])
	}
}

func TestDoctorJSONFailureEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	// Empty directory: no built server entrypoint, so the checks run in
	// the CLI against the stubbed local table.
	t.Chdir(t.TempDir())

	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{
			"config": func() error { return nil },
			"db":     func() error { return errFakeBatteryFailure },
		}
	}

	var human bytes.Buffer
	err := runDoctorWith(DoctorConfig{Out: &human, JSON: true})
	if err == nil {
		t.Fatal("runDoctorWith JSON with a failing battery: got nil error, want non-nil for nonzero exit")
	}

	if exitCodeFor(err) != ExitError {
		t.Fatalf("exitCodeFor(err) = %d, want %d", exitCodeFor(err), ExitError)
	}

	if human.String() != "" {
		t.Fatalf("human output = %q in JSON mode, want only the envelope", human.String())
	}

	env := mustDecodeEnvelope(t, jbuf)
	if env.OK || env.Command != "doctor" || env.ExitCode != ExitError {
		t.Fatalf("envelope = %+v, want failed doctor envelope", env)
	}

	data := envelopeData(t, env)
	if failed, ok := data["failed"].(float64); !ok || failed != 1 {
		t.Fatalf("data.failed = %v, want 1", data["failed"])
	}

	if env.Error == "" {
		t.Fatal("failed envelope carries no error message")
	}
}

func TestDoctorJSONSuccessEnvelope(t *testing.T) {
	jbuf := mustJSONMode(t)

	prev := doctorChecksFor
	t.Cleanup(func() { doctorChecksFor = prev })
	doctorChecksFor = func(_ *container.Container, _ *config.Config) map[string]func() error {
		return map[string]func() error{
			"config": func() error { return nil },
			"db":     func() error { return nil },
		}
	}

	var human bytes.Buffer
	if err := runDoctorWith(DoctorConfig{Out: &human, JSON: true}); err != nil {
		t.Fatalf("runDoctorWith JSON all-OK: %v", err)
	}

	if human.String() != "" {
		t.Fatalf("human output = %q in JSON mode, want only the envelope", human.String())
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "doctor" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok doctor envelope", env)
	}
}

func TestCheckJSONFailureCarriesDiagnostics(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	brokenPath := writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)

	var human bytes.Buffer
	err := runCheckWith(CheckConfig{Files: []string{brokenPath}, Out: &human, JSON: true})
	if err == nil {
		t.Fatal("runCheckWith JSON on broken schema: got nil error, want non-nil")
	}

	if !errors.Is(err, errJSONEmitted) {
		t.Fatalf("err = %v, want it marked already-emitted so main exits without a second envelope", err)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if env.OK || env.Command != "check" || env.ExitCode != ExitError {
		t.Fatalf("envelope = %+v, want failed check envelope", env)
	}

	if len(env.Diagnostics) == 0 {
		t.Fatal("failed check envelope carries no diagnostics")
	}
}

func TestCheckJSONSuccessContract(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var human bytes.Buffer
	if err := runCheckWith(CheckConfig{Files: []string{schemaPath}, Out: &human, JSON: true}); err != nil {
		t.Fatalf("runCheckWith JSON: %v", err)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "check" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok check envelope", env)
	}

	data := envelopeData(t, env)
	if valid, ok := data["valid"].(bool); !ok || !valid {
		t.Fatalf("data.valid = %v, want true", data["valid"])
	}
}

func TestExplainJSONSuccessContract(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var human bytes.Buffer
	cfg := ExplainConfig{OpPath: "UserService.GetUser", Files: []string{schemaPath}, Out: &human, JSON: true}
	if err := runExplainWith(cfg); err != nil {
		t.Fatalf("runExplainWith JSON: %v", err)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "explain" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok explain envelope", env)
	}

	data := envelopeData(t, env)
	if data["operation"] != "GetUser" || data["service"] != "UserService" {
		t.Fatalf("data = %v, want GetUser/UserService", data)
	}
}

func TestCompileJSONSuccessContract(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)
	outDir := filepath.Join(dir, "generated")

	var human bytes.Buffer
	cfg := CompileConfig{Files: []string{schemaPath}, Backends: "atlas", OutDir: outDir, Out: &human, JSON: true}
	if err := runCompileWith(cfg); err != nil {
		t.Fatalf("runCompileWith JSON: %v", err)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "compile" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok compile envelope", env)
	}
}

func TestBoundariesJSONSuccessContract(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var human bytes.Buffer
	if err := runCheckBoundariesWith(BoundariesConfig{Files: []string{schemaPath}, Out: &human, JSON: true}); err != nil {
		t.Fatalf("runCheckBoundariesWith JSON: %v", err)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "check-boundaries" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok boundaries envelope", env)
	}
}

func TestBreakingJSONNoDiffContract(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old.zen", inspectUserSchema)
	newPath := writeInspectFixture(t, dir, "new.zen", inspectUserSchema)

	var human bytes.Buffer
	cfg := BreakingConfig{OldFiles: []string{oldPath}, NewFiles: []string{newPath}, Out: &human, JSON: true}
	if err := runBreakingWith(cfg); err != nil {
		t.Fatalf("runBreakingWith JSON no-diff: %v", err)
	}

	env := mustDecodeEnvelope(t, jbuf)
	if !env.OK || env.Command != "breaking" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want ok breaking envelope", env)
	}
}

func TestBreakingJSONBreakingChangeContract(t *testing.T) {
	jbuf := mustJSONMode(t)

	dir := t.TempDir()
	oldPath := writeInspectFixture(t, dir, "old.zen", inspectUserSchema)
	newPath := writeInspectFixture(t, dir, "new.zen", "entity User {\n  id: uuid @primary\n}\n")

	var human bytes.Buffer
	cfg := BreakingConfig{OldFiles: []string{oldPath}, NewFiles: []string{newPath}, Out: &human, JSON: true}
	err := runBreakingWith(cfg)
	if err == nil {
		t.Fatal("runBreakingWith JSON with removed RPC: got nil error, want non-nil")
	}

	env := mustDecodeEnvelope(t, jbuf)
	if env.OK || env.Command != "breaking" || env.ExitCode != ExitError {
		t.Fatalf("envelope = %+v, want failed breaking envelope", env)
	}

	data := envelopeData(t, env)
	if breaking, ok := data["breaking"].(float64); !ok || breaking == 0 {
		t.Fatalf("data.breaking = %v, want nonzero", data["breaking"])
	}
}

func TestUnknownFlagsRejectedWithSuggestion(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
		hint string
	}{
		{"routes", func() error { return runRoutes([]string{"--bogus"}) }, "unknown flag"},
		{"graph", func() error { return runGraph([]string{"--bogus"}) }, "unknown flag"},
		{"check", func() error { return runCheck([]string{"--bogus"}) }, "unknown flag"},
		{"boundaries", func() error { return runCheckBoundaries([]string{"--bogus"}) }, "unknown flag"},
		{"config show", func() error { return runConfigShow([]string{"--confg"}) }, "unknown flag"},
		{"doctor", func() error { return runDoctor([]string{"--confg"}) }, "unknown flag"},
		{"breaking", func() error { return runBreaking([]string{"--bogus", "--", "a.zen"}) }, "unknown flag"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatalf("%s --bogus: got nil error, want unknown-flag rejection", tc.name)
			}

			if !strings.Contains(err.Error(), "unknown flag") {
				t.Fatalf("%s: error = %q, want it to report an unknown flag", tc.name, err.Error())
			}

			if tc.hint != "unknown flag" && !strings.Contains(err.Error(), tc.hint) {
				t.Fatalf("%s: error = %q, want suggestion %q", tc.name, err.Error(), tc.hint)
			}
		})
	}
}
