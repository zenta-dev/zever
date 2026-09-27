package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

const inspectExplainSchema = `entity User {
	id: uuid @primary
	email: string @unique
}

service UserService {
	rpc GetUser(id: uuid) -> User {
		http: GET "/v1/users/{id}"
		auth: required(roles: {admin})
		permission: check("owns", resource: User, owner_field: id)
		errors: { not_found, permission_denied("no access") }
	}
}
`

func TestRunExplainUsage(t *testing.T) {
	if err := runExplain(nil); err == nil {
		t.Fatalf("expected error when no args given, got nil")
	}

	if err := runExplain([]string{"UserService.GetUser"}); err == nil {
		t.Fatalf("expected error when no input files given, got nil")
	}
}

func TestRunExplainFindsOperation(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runExplain([]string{"UserService.GetUser", schemaPath})
	})

	if runErr != nil {
		t.Fatalf("runExplain: %v", runErr)
	}

	wantContains := []string{
		"(root).UserService.GetUser",
		"user.zen:",
		"transports: gRPC, HTTP GET /v1/users/{id}",
		"auth:       required (roles: admin)",
		"permission: check(\"owns\", resource: User)",
		"errors:     NOT_FOUND, PERMISSION_DENIED",
	}

	for _, want := range wantContains {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want it to contain %q", output, want)
		}
	}
}

func TestRunExplainModuleQualifiedPath(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runExplain([]string{"(root).UserService.GetUser", schemaPath})
	})

	// The root module's Name is "", so a literal "(root)" segment will not
	// match any module and should fail to resolve.
	if runErr == nil {
		t.Fatalf("expected error for unresolvable module-qualified path, got output %q", output)
	}
}

func TestRunExplainUnknownOperation(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	err := runExplain([]string{"UserService.Bogus", schemaPath})
	if err == nil {
		t.Fatalf("expected error for unknown operation, got nil")
	}

	if !strings.Contains(err.Error(), "no operation matches") {
		t.Fatalf("error = %q, want it to mention 'no operation matches'", err.Error())
	}
}

func TestRunExplainSuggestsClosestMatch(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	err := runExplain([]string{"UserService.GetUsr", schemaPath})
	if err == nil {
		t.Fatalf("expected error for near-miss operation, got nil")
	}

	if !strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error = %q, want it to suggest a closest match", err.Error())
	}
}

func TestRunExplainInvalidPathShape(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	err := runExplain([]string{"JustOneSegment", schemaPath})
	if err == nil {
		t.Fatalf("expected error for a one-segment path, got nil")
	}
}

func TestRunExplainCompileError(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)

	if err := runExplain([]string{"Service.Op", schemaPath}); err == nil {
		t.Fatalf("expected error for broken schema, got nil")
	}
}

func TestRunExplainWithCoreWritesToOut(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	var out bytes.Buffer
	err := runExplainWith(ExplainConfig{OpPath: "UserService.GetUser", Files: []string{schemaPath}, Out: &out})
	if err != nil {
		t.Fatalf("runExplainWith: %v", err)
	}

	if !strings.Contains(out.String(), "UserService.GetUser") {
		t.Fatalf("output = %q, want the qualified operation name", out.String())
	}
}

func TestRunExplainWithCoreEmptyFiles(t *testing.T) {
	var out bytes.Buffer
	if err := runExplainWith(ExplainConfig{OpPath: "UserService.GetUser", Out: &out}); err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
}

func TestRunExplainModuleQualifiedSuccess(t *testing.T) {
	dir := t.TempDir()
	billingPath := writeInspectFixture(t, dir, "schema/billing/order.zen", `entity Order {
	id: uuid @primary
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		auth: required
	}
}
`)
	writeInspectFixture(t, dir, "schema/shipping/ship.zen", `entity Shipment {
	id: uuid @primary
}
`)

	var out bytes.Buffer
	err := runExplainWith(ExplainConfig{
		OpPath: "billing.OrderService.GetOrder",
		Files:  []string{billingPath, dir + "/schema/shipping/ship.zen"},
		Out:    &out,
	})
	if err != nil {
		t.Fatalf("runExplainWith module-qualified: %v", err)
	}

	if !strings.Contains(out.String(), "billing.OrderService.GetOrder") {
		t.Fatalf("output = %q, want the module-qualified name", out.String())
	}
}

func TestRunExplainNotFoundWithoutSuggestion(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	err := runExplain([]string{"Zzz.Qqq", schemaPath})
	if err == nil {
		t.Fatalf("expected error for unknown operation, got nil")
	}

	if strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error = %q, want no suggestion for a far-off path", err.Error())
	}
}

func TestExplainHelperSummaries(t *testing.T) {
	if got := explainTransports(nil); got != "gRPC" {
		t.Fatalf("explainTransports(nil) = %q, want gRPC", got)
	}

	if got := explainTransports([]ir.Transport{ir.HTTPTransport{Method: "POST", Path: "/v1/x"}}); got != "HTTP POST /v1/x" {
		t.Fatalf("explainTransports(http) = %q", got)
	}

	if got := explainAuth(nil); got != "none" {
		t.Fatalf("explainAuth(nil) = %q, want none", got)
	}

	if got := explainAuth(&ir.AuthPolicy{}); got != "required" {
		t.Fatalf("explainAuth(empty) = %q, want required", got)
	}

	if got := explainPermission(nil); got != "none" {
		t.Fatalf("explainPermission(nil) = %q, want none", got)
	}

	if got := explainPermission(&ir.PermissionCheck{Check: "owns"}); got != `check("owns", resource: ?)` {
		t.Fatalf("explainPermission(no resource) = %q", got)
	}

	if got := explainErrors(nil); got != "none" {
		t.Fatalf("explainErrors(nil) = %q, want none", got)
	}
}

func TestParseExplainPathShapes(t *testing.T) {
	tests := []struct {
		name        string
		path        string
		wantModule  string
		wantService string
		wantOp      string
		wantErr     bool
	}{
		{"two segments", "Svc.Op", "", "Svc", "Op", false},
		{"three segments", "Mod.Svc.Op", "Mod", "Svc", "Op", false},
		{"one segment", "Op", "", "", "", true},
		{"four segments", "A.B.C.D", "", "", "", true},
		{"empty", "", "", "", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mod, svc, op, err := parseExplainPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for path %q, got nil", tt.path)
				}

				return
			}

			if err != nil {
				t.Fatalf("parseExplainPath(%q): %v", tt.path, err)
			}

			if mod != tt.wantModule || svc != tt.wantService || op != tt.wantOp {
				t.Fatalf("got (%q, %q, %q), want (%q, %q, %q)", mod, svc, op, tt.wantModule, tt.wantService, tt.wantOp)
			}
		})
	}
}

func TestRunExplainAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/app.zen", inspectExplainSchema)
	t.Chdir(dir)
	t.Setenv("ZEVER_NO_HINT", "1")

	var runErr error
	output := inspectCaptureOutput(t, func() {
		runErr = runExplain([]string{"UserService.GetUser"})
	})
	if runErr != nil {
		t.Fatalf("runExplain auto-discovered: %v (output = %q)", runErr, output)
	}
	if !strings.Contains(output, "UserService.GetUser") {
		t.Fatalf("output = %q, want operation name", output)
	}
}

func TestPrintExplainColored(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectExplainSchema)

	prev := colorEnabled
	colorEnabled = true
	t.Cleanup(func() { colorEnabled = prev })

	var out bytes.Buffer
	if err := runExplainWith(ExplainConfig{OpPath: "UserService.GetUser", Files: []string{schemaPath}, Out: &out}); err != nil {
		t.Fatalf("runExplainWith colored: %v", err)
	}
	if !strings.Contains(out.String(), "UserService.GetUser") {
		t.Fatalf("output = %q, want operation name", out.String())
	}
}

func TestExplainNotFoundWithModulesSuggestsQualified(t *testing.T) {
	dir := t.TempDir()
	billingPath := writeInspectFixture(t, dir, "schema/billing/order.zen", `entity Order {
	id: uuid @primary
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		auth: required
	}
}
`)
	writeInspectFixture(t, dir, "schema/shipping/ship.zen", `entity Shipment {
	id: uuid @primary
}
`)

	err := runExplain([]string{"billing.OrderService.GetOrde", billingPath, dir + "/schema/shipping/ship.zen"})
	if err == nil {
		t.Fatalf("expected error for near-miss qualified path, got nil")
	}
	if !strings.Contains(err.Error(), "did you mean") {
		t.Fatalf("error = %q, want qualified suggestion", err.Error())
	}
}
