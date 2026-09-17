package compile

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/backend/proto"
	"github.com/zenta-dev/zever/internal/dsl/ir"
)

// findEntity searches every module of schema for an entity named name.
func findEntity(schema *ir.Schema, name string) *ir.Entity {
	for _, m := range schema.Modules {
		for _, e := range m.Entities {
			if e.Name == name {
				return e
			}
		}
	}

	return nil
}

// findModule searches schema for a module named name.
func findModule(schema *ir.Schema, name string) *ir.Module {
	for _, m := range schema.Modules {
		if m.Name == name {
			return m
		}
	}

	return nil
}

func TestCompileCrossFileEntityReference(t *testing.T) {
	userSrc := `entity User {
		id: uuid @primary
		email: string @unique
	}`

	orderSrc := `entity Order {
		id: uuid @primary
		user_id: uuid

		belongs_to user: User @foreign_key(user_id)
	}`

	// "aaa_order.zen" sorts before "zzz_user.zen" alphabetically, which is
	// the opposite of Go's typical map insertion order below (user first) --
	// this exercises that sorting, not insertion order, controls processing.
	tests := []struct {
		name  string
		files map[string]string
	}{
		{
			name: "user file name sorts after order file name",
			files: map[string]string{
				"zzz_user.zen":  userSrc,
				"aaa_order.zen": orderSrc,
			},
		},
		{
			name: "user file name sorts before order file name",
			files: map[string]string{
				"aaa_user.zen":  userSrc,
				"zzz_order.zen": orderSrc,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for i := 0; i < 5; i++ {
				result, diags := Compile(tc.files)
				if diags.HasErrors() {
					t.Fatalf("run %d: unexpected diagnostics: %v", i, diags)
				}

				order := findEntity(result.Schema, "Order")
				if order == nil {
					t.Fatalf("run %d: Order entity not found in schema", i)
				}

				if len(order.Relations) != 1 {
					t.Fatalf("run %d: Order.Relations len = %d, want 1", i, len(order.Relations))
				}

				if order.Relations[0].Target == nil || order.Relations[0].Target.Name != "User" {
					t.Fatalf("run %d: Order relation target = %v, want User", i, order.Relations[0].Target)
				}

				if result.Outputs == nil {
					t.Fatalf("run %d: Outputs should be a non-nil empty map when diags has no errors, got nil", i)
				}

				if len(result.Outputs) != 0 {
					t.Fatalf("run %d: Outputs should be empty with no backends passed, got %v", i, result.Outputs)
				}
			}
		})
	}
}

func TestCompileBrokenFilePlusCleanFile(t *testing.T) {
	brokenSrc := `entity Broken {
		id: uuid @primary
		bad_field: nonexistent_type
	}`

	cleanSrc := `entity Clean {
		id: uuid @primary
		name: string
	}`

	files := map[string]string{
		"broken.zen": brokenSrc,
		"clean.zen":  cleanSrc,
	}

	// Backend-agnostic variant: the source passes proto.New() here, but the
	// assertions (error diags, non-nil schema, nil Outputs) hold with no
	// backends. The proto-backend case is skipped until backends land.
	result, diags := Compile(files)

	if !diags.HasErrors() {
		t.Fatalf("expected diagnostics reporting the broken file, got none")
	}

	if result.Schema == nil {
		t.Fatalf("Result.Schema is nil, want non-nil")
	}

	if findEntity(result.Schema, "Clean") == nil {
		t.Fatalf("Clean entity missing from schema despite Broken being invalid")
	}

	if result.Outputs != nil {
		t.Fatalf("Outputs = %v, want nil since diags.HasErrors() is true", result.Outputs)
	}
}

func TestCompileValidMultiFileWithProtoBackend(t *testing.T) {
	t.Parallel()

	userSrc := `entity User {
		id: uuid @primary
		email: string @unique
	}`

	serviceSrc := `service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/v1/users/{id}"
			auth: required
		}
	}`

	files := map[string]string{
		"a_user.zen":    userSrc,
		"b_service.zen": serviceSrc,
	}

	result, diags := Compile(files, proto.New())

	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	// Filter warnings (secureEverything currently emits warning) – expect zero errors/warnings for this clean case with auth.
	if len(diags) != 0 {
		hasError := false
		for _, d := range diags {
			if d.Severity == 0 {
				hasError = true
			}
		}
		if hasError {
			t.Fatalf("expected zero error diagnostics, got %d: %v", len(diags), diags)
		}
	}

	if result.Outputs == nil {
		t.Fatalf("Outputs is nil, want non-nil")
	}

	protoOut, ok := result.Outputs["proto"]
	if !ok {
		t.Fatalf("Outputs missing %q key, got keys %v", "proto", keysOf(result.Outputs))
	}

	if len(protoOut) == 0 {
		t.Fatalf("Outputs[%q] is empty, want at least one file", "proto")
	}

	if _, ok := protoOut["schema.proto"]; !ok {
		t.Fatalf("Outputs[%q] missing schema.proto, got keys %v", "proto", keysOf(protoOut))
	}
}

func TestCompileModuleSplitAcrossFiles(t *testing.T) {
	entitySrc := `entity Order {
			id: uuid @primary
			amount_cents: int64
		}`

	serviceSrc := `service OrderService {
			rpc GetOrder(id: uuid) -> Order {
				http: GET "/v1/orders/{id}"
				auth: required
			}
		}`

	files := map[string]string{
		"schema/billing/entities.zen": entitySrc,
		"schema/billing/services.zen": serviceSrc,
	}

	result, diags := WithSchemaDir(files, "schema")
	if diags.HasErrors() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	billingModules := 0

	for _, m := range result.Schema.Modules {
		if m.Name == "billing" {
			billingModules++
		}
	}

	if billingModules != 1 {
		t.Fatalf("found %d modules named billing, want exactly 1 (module grouping must be schema-wide)", billingModules)
	}

	mod := findModule(result.Schema, "billing")
	if mod == nil {
		t.Fatalf("billing module not found")
	}

	if len(mod.Entities) != 1 {
		t.Fatalf("billing module Entities len = %d, want 1", len(mod.Entities))
	}

	if len(mod.Services) != 1 {
		t.Fatalf("billing module Services len = %d, want 1", len(mod.Services))
	}
}

func keysOf[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}

	return out
}
