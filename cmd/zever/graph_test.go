package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestRunGraphNoFiles(t *testing.T) {
	err := runGraph(nil)
	if !errors.Is(err, errNoInputFiles) {
		t.Fatalf("error = %v, want errNoInputFiles", err)
	}
}

func TestRunGraphMonolithEntityRelations(t *testing.T) {
	dir := t.TempDir()
	billingPath := writeInspectFixture(t, dir, "schema/billing/a.zen", inspectCleanModulesBilling)
	shippingPath := writeInspectFixture(t, dir, "schema/shipping/b.zen", inspectCleanModulesShipping)

	var runErr error

	output := inspectCaptureOutput(t, func() {
		runErr = runGraph([]string{billingPath, shippingPath})
	})

	if runErr != nil {
		t.Fatalf("runGraph: %v (output = %q)", runErr, output)
	}

	wants := []string{
		"```mermaid",
		"graph TD",
		"%% module: billing",
		`Order["Order"]`,
		`Customer["Customer"]`,
		"Order -->|belongs_to| Customer",
		"%% module: shipping",
		`Shipment["Shipment"]`,
		"graph LR",
		`billing["billing"]`,
		`shipping["shipping"]`,
	}
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want it to contain %q", output, want)
		}
	}

	// No cross-module RPC returns in a clean schema, so no module edges.
	if strings.Contains(output, "-->|rpc|") {
		t.Fatalf("output = %q, want no module-to-module rpc edges", output)
	}
}

func TestRunGraphCrossModuleRPCEdge(t *testing.T) {
	dir := t.TempDir()
	billingPath := writeInspectFixture(t, dir, "schema/billing/a.zen", inspectCrossModuleRPCBilling)
	shippingPath := writeInspectFixture(t, dir, "schema/shipping/b.zen", inspectCrossModuleRPCShipping)

	var out bytes.Buffer

	runErr := runGraphWith(GraphConfig{
		Files: []string{billingPath, shippingPath},
		Out:   &out,
	})

	// A cross-module RPC return is a resolver error, so graph still exits
	// non-zero while emitting the best-effort diagram.
	if runErr == nil {
		t.Fatalf("expected a non-nil error for a schema with a cross-module RPC return, got nil")
	}

	output := out.String()
	wants := []string{
		"graph LR",
		`billing["billing"]`,
		`shipping["shipping"]`,
		"billing -->|rpc| shipping",
	}
	for _, want := range wants {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want it to contain %q", output, want)
		}
	}
}

func TestRunGraphImplicitModuleLabeledRoot(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "user.zen", inspectUserSchema)

	var out bytes.Buffer
	if err := runGraphWith(GraphConfig{Files: []string{schemaPath}, Out: &out}); err != nil {
		t.Fatalf("runGraphWith: %v", err)
	}

	output := out.String()
	for _, want := range []string{"%% module: root", `User["User"]`, `root["root"]`} {
		if !strings.Contains(output, want) {
			t.Fatalf("output = %q, want it to contain %q", output, want)
		}
	}
}

func TestRunGraphWithCoreEmptyFiles(t *testing.T) {
	var out bytes.Buffer
	if err := runGraphWith(GraphConfig{Out: &out}); err == nil {
		t.Fatalf("expected error for empty file list, got nil")
	}
}

func TestModuleLabel(t *testing.T) {
	if got := moduleLabel(nil); got != "root" {
		t.Fatalf("moduleLabel(nil) = %q, want root", got)
	}
}

func TestRunGraphDedupsModuleEdges(t *testing.T) {
	dir := t.TempDir()
	billingPath := writeInspectFixture(t, dir, "schema/billing/a.zen", `entity Order {
	id: uuid @primary
}

service OrderService {
	rpc GetShipment(id: uuid) -> Shipment {
		auth: required
	}
	rpc TrackShipment(id: uuid) -> Shipment {
		auth: required
	}
}
`)
	shippingPath := writeInspectFixture(t, dir, "schema/shipping/b.zen", `entity Shipment {
	id: uuid @primary
}
`)

	var out bytes.Buffer
	runErr := runGraphWith(GraphConfig{
		Files: []string{billingPath, shippingPath},
		Out:   &out,
	})
	if runErr == nil {
		t.Fatalf("expected a non-nil error for cross-module RPC returns, got nil")
	}

	if got := strings.Count(out.String(), "billing -->|rpc| shipping"); got != 1 {
		t.Fatalf("output has %d module edges, want exactly 1:\n%s", got, out.String())
	}
}

func TestRunGraphNilRelationTargetStillPrints(t *testing.T) {
	dir := t.TempDir()
	schemaPath := writeInspectFixture(t, dir, "a.zen", `entity A {
	id: uuid @primary
	owner_id: uuid
	belongs_to owner: Nope @foreign_key(owner_id)
}
`)

	var out bytes.Buffer
	runErr := runGraphWith(GraphConfig{Files: []string{schemaPath}, Out: &out})
	if runErr == nil {
		t.Fatalf("expected a non-nil error for an unresolvable relation target, got nil")
	}

	if !strings.Contains(out.String(), `A["A"]`) {
		t.Fatalf("output = %q, want the entity node best-effort", out.String())
	}
}

func TestRelationKindName(t *testing.T) {
	cases := []struct {
		kind ir.RelationKind
		want string
	}{
		{ir.HasMany, "has_many"},
		{ir.HasOne, "has_one"},
		{ir.BelongsTo, "belongs_to"},
		{ir.ManyToMany, "many_to_many"},
		{ir.RelationKind(999), "unknown"},
	}

	for _, tc := range cases {
		if got := relationKindName(tc.kind); got != tc.want {
			t.Fatalf("relationKindName(%d) = %q, want %q", int(tc.kind), got, tc.want)
		}
	}
}

func TestRunGraphAutoDiscovery(t *testing.T) {
	dir := t.TempDir()
	writeInspectFixture(t, dir, "schema/app.zen", inspectUserSchema)
	t.Chdir(dir)
	t.Setenv("ZEVER_NO_HINT", "1")

	var runErr error
	output := inspectCaptureOutput(t, func() {
		runErr = runGraph(nil)
	})
	if runErr != nil {
		t.Fatalf("runGraph auto-discovered: %v (output = %q)", runErr, output)
	}
	if !strings.Contains(output, "```mermaid") {
		t.Fatalf("output = %q, want mermaid block", output)
	}
}

func TestPrintEntityGraphNilTarget(t *testing.T) {
	var out bytes.Buffer
	m := &ir.Module{Name: "billing"}
	m.Entities = []*ir.Entity{{
		Name:      "Order",
		Relations: []*ir.Relation{{Target: nil}},
	}}
	printEntityGraph(&out, m)
	if !strings.Contains(out.String(), `Order["Order"]`) {
		t.Fatalf("output = %q, want entity node", out.String())
	}
}
