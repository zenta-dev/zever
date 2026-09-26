package zenorm

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/ir"
)

func TestModuleNaming(t *testing.T) {
	t.Run("nil module maps to app", func(t *testing.T) {
		pkg, path, err := moduleNaming(nil)
		if err != nil || pkg != "app" || path != "orm/gen/app/app.go" {
			t.Fatalf("moduleNaming(nil) = (%q, %q, %v), want (app, orm/gen/app/app.go, nil)", pkg, path, err)
		}
	})

	t.Run("unnamed module maps to app", func(t *testing.T) {
		pkg, path, err := moduleNaming(&ir.Module{})
		if err != nil || pkg != "app" || path != "orm/gen/app/app.go" {
			t.Fatalf("moduleNaming(unnamed) = (%q, %q, %v), want (app, orm/gen/app/app.go, nil)", pkg, path, err)
		}
	})

	t.Run("named module maps to its snake name", func(t *testing.T) {
		pkg, path, err := moduleNaming(&ir.Module{Name: "billing"})
		if err != nil || pkg != "billing" || path != "orm/gen/billing/billing.go" {
			t.Fatalf("moduleNaming(billing) = (%q, %q, %v), want (billing, orm/gen/billing/billing.go, nil)", pkg, path, err)
		}
	})

	t.Run("pascal module name snakes", func(t *testing.T) {
		pkg, path, err := moduleNaming(&ir.Module{Name: "Billing"})
		if err != nil || pkg != "billing" || path != "orm/gen/billing/billing.go" {
			t.Fatalf("moduleNaming(Billing) = (%q, %q, %v), want (billing, orm/gen/billing/billing.go, nil)", pkg, path, err)
		}
	})

	t.Run("traversal-shaped name is rejected", func(t *testing.T) {
		_, _, err := moduleNaming(&ir.Module{Name: "../../etc/cron.d"})
		if err == nil {
			t.Fatal("moduleNaming(traversal name) succeeded, want an error")
		}
	})
}

func TestModuleLabel(t *testing.T) {
	if got := moduleLabel(nil); got != "" {
		t.Fatalf("moduleLabel(nil) = %q, want empty", got)
	}

	if got := moduleLabel(&ir.Module{}); got != "" {
		t.Fatalf("moduleLabel(unnamed) = %q, want empty", got)
	}

	if got := moduleLabel(&ir.Module{Name: "billing"}); got != "billing" {
		t.Fatalf("moduleLabel(billing) = %q, want billing", got)
	}
}

func TestPascalModuleLabel(t *testing.T) {
	if got := pascalModuleLabel(""); got != "application" {
		t.Fatalf("pascalModuleLabel(empty) = %q, want application", got)
	}

	if got := pascalModuleLabel("billing"); got != "Billing" {
		t.Fatalf("pascalModuleLabel(billing) = %q, want Billing", got)
	}
}

// TestGenerateNamedModule proves a named module renders to its own
// orm/gen/<module> path with a matching package clause.
func TestGenerateNamedModule(t *testing.T) {
	src := `entity Invoice {
		id: uuid @primary
		number: string @unique
	}`

	schema := compileSchema(t, src)
	schema.Modules[0].Name = "billing"

	out, err := New().Generate(schema)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	const path = "orm/gen/billing/billing.go"

	content, ok := out[path]
	if !ok {
		t.Fatalf("Generate output missing %q, got keys %v", path, keys(out))
	}

	checkGolden(t, "named_module", content)
	validateGoSyntax(t, path, content)
}
