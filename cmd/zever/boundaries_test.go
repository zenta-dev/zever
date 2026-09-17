package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// inspectCrossModuleRelationBilling puts Order in `billing` and Target in
// `shipping` and declares a direct relation across that boundary.
const inspectCrossModuleRelationBilling = `entity Order {
	id: uuid @primary
	shipment_id: uuid
	belongs_to shipment: Shipment @foreign_key(shipment_id)
}
`
const inspectCrossModuleRelationShipping = `entity Shipment {
	id: uuid @primary
}
`

// crossModuleRPCSchema returns an entity owned by another module from a
// service's RPC.
const inspectCrossModuleRPCBilling = `entity Order {
	id: uuid @primary
}

service OrderService {
	rpc GetShipment(id: uuid) -> Shipment {
		http: GET "/v1/shipments/{id}"
		auth: required
	}
}
`
const inspectCrossModuleRPCShipping = `entity Shipment {
	id: uuid @primary
}
`

// crossModulePermissionSchema names another module's entity as the permission
// resource of an RPC.
const inspectCrossModulePermissionBilling = `entity Order {
	id: uuid @primary
}

service OrderService {
	rpc GetOrder(id: uuid) -> Order {
		http: GET "/v1/orders/{id}"
		auth: required
		permission: check("owner", resource: Shipment, owner_field: user_id)
	}
}
`
const inspectCrossModulePermissionShipping = `entity Shipment {
	id: uuid @primary
	user_id: uuid
}
`

// cleanModulesSchema keeps every reference inside its own module.
const inspectCleanModulesBilling = `entity Order {
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
		permission: check("owner", resource: Customer, owner_field: user_id)
	}
}
`
const inspectCleanModulesShipping = `entity Shipment {
	id: uuid @primary
}
`

func TestRunCheckBoundariesNoFiles(t *testing.T) {
	err := runCheckBoundaries(nil)
	if !errors.Is(err, errNoInputFiles) {
		t.Fatalf("error = %v, want errNoInputFiles", err)
	}
}

func TestRunCheckBoundariesReportsViolations(t *testing.T) {
	cases := []struct {
		name     string
		billing  string
		shipping string
		want     string
	}{
		{"relation", inspectCrossModuleRelationBilling, inspectCrossModuleRelationShipping, "crosses module boundary (billing -> shipping)"},
		{"rpc return", inspectCrossModuleRPCBilling, inspectCrossModuleRPCShipping, "returns Shipment from a different module (billing -> shipping)"},
		{"permission resource", inspectCrossModulePermissionBilling, inspectCrossModulePermissionShipping, "permission resource Shipment is from a different module (billing -> shipping)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			billingPath := writeInspectFixture(t, dir, "schema/billing/a.zen", tc.billing)
			shippingPath := writeInspectFixture(t, dir, "schema/shipping/b.zen", tc.shipping)

			var out bytes.Buffer

			runErr := runCheckBoundariesWith(BoundariesConfig{
				Files: []string{billingPath, shippingPath},
				Out:   &out,
			})

			if runErr == nil {
				t.Fatalf("expected a non-nil error (non-zero exit) for a cross-module violation, got nil; output = %q", out.String())
			}

			if !strings.Contains(runErr.Error(), "cross-module violation") {
				t.Fatalf("error = %q, want it to mention cross-module violation(s)", runErr.Error())
			}

			if !strings.Contains(out.String(), "cross-module boundary violations:") {
				t.Fatalf("output = %q, want the violations section header", out.String())
			}

			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("output = %q, want it to contain %q", out.String(), tc.want)
			}
		})
	}
}

func TestRunCheckBoundariesCleanSchema(t *testing.T) {
	dir := t.TempDir()
	billingPath := writeInspectFixture(t, dir, "schema/billing/a.zen", inspectCleanModulesBilling)
	shippingPath := writeInspectFixture(t, dir, "schema/shipping/b.zen", inspectCleanModulesShipping)

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runCheckBoundaries([]string{billingPath, shippingPath})
	})

	if runErr != nil {
		t.Fatalf("runCheckBoundaries: %v (output = %q)", runErr, output)
	}

	const want = "zever check-boundaries: no cross-module violations"
	if !strings.Contains(output, want) {
		t.Fatalf("output = %q, want it to contain %q", output, want)
	}
}

func TestRunCheckBoundariesCleanMonolith(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var out bytes.Buffer
	if err := runCheckBoundariesWith(BoundariesConfig{Files: []string{schemaPath}, Out: &out}); err != nil {
		t.Fatalf("runCheckBoundariesWith: %v (output = %q)", err, out.String())
	}

	if !strings.Contains(out.String(), "no cross-module violations") {
		t.Fatalf("output = %q, want the success message", out.String())
	}
}

func TestRunCheckBoundariesCompileErrorStillReported(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "broken.zen", inspectBrokenSchema)

	// A schema that fails to compile has no cross-module violations, so the
	// command succeeds on its own terms; the diagnostics still go to stderr
	// via printDiagnostics.
	var out bytes.Buffer
	if err := runCheckBoundariesWith(BoundariesConfig{Files: []string{schemaPath}, Out: &out}); err != nil {
		t.Fatalf("runCheckBoundariesWith: %v", err)
	}

	if !strings.Contains(out.String(), "no cross-module violations") {
		t.Fatalf("output = %q, want the success message", out.String())
	}
}

func TestRunCheckBoundariesWithCoreEmptyFiles(t *testing.T) {
	var out bytes.Buffer
	if err := runCheckBoundariesWith(BoundariesConfig{Out: &out}); err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
}

func TestRunCheckBoundariesAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/billing/a.zen", inspectCleanModulesBilling)
	writeInspectFixture(t, dir, "schema/shipping/b.zen", inspectCleanModulesShipping)
	t.Chdir(dir)
	t.Setenv("ZEVER_NO_HINT", "1")

	var runErr error
	output := inspectCaptureOutput(t, func() {
		runErr = runCheckBoundaries(nil)
	})
	if runErr != nil {
		t.Fatalf("runCheckBoundaries auto-discovered: %v (output = %q)", runErr, output)
	}
	if !strings.Contains(output, "no cross-module violations") {
		t.Fatalf("output = %q, want success message", output)
	}
}
