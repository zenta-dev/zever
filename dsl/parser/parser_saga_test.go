package parser

import (
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
)

func firstSaga(t *testing.T, file *ast.File) *ast.SagaDecl {
	t.Helper()

	for _, d := range file.Decls {
		if s, ok := d.(*ast.SagaDecl); ok {
			return s
		}
	}

	t.Fatalf("no SagaDecl found among %d top-level decls", len(file.Decls))

	return nil
}

func assertHasMsg(t *testing.T, msgs []string, want string) {
	t.Helper()

	for _, m := range msgs {
		if strings.Contains(m, want) {
			return
		}
	}

	t.Fatalf("no diagnostic contains %q; got %v", want, msgs)
}

func TestParseSagaDecl(t *testing.T) {
	t.Parallel()

	src := `saga CheckoutSaga {
		step reserve {
			execute: InventoryService.Reserve
			compensate: InventoryService.Release
		}
		step charge {
			execute: PaymentService.Charge
			compensate: PaymentService.Refund
			pivot: true
		}
	}`

	file := mustParseClean(t, src)

	saga := firstSaga(t, file)
	if saga.Name != "CheckoutSaga" {
		t.Fatalf("saga name = %q, want CheckoutSaga", saga.Name)
	}

	if len(saga.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(saga.Steps))
	}

	reserve := saga.Steps[0]
	if reserve.Name != "reserve" {
		t.Errorf("step[0].Name = %q, want reserve", reserve.Name)
	}

	if reserve.Execute == nil || reserve.Execute.Service != "InventoryService" || reserve.Execute.RPC != "Reserve" {
		t.Errorf("step[0].Execute = %+v, want InventoryService.Reserve", reserve.Execute)
	}

	if reserve.Compensate == nil || reserve.Compensate.Service != "InventoryService" || reserve.Compensate.RPC != "Release" {
		t.Errorf("step[0].Compensate = %+v, want InventoryService.Release", reserve.Compensate)
	}

	if reserve.Pivot {
		t.Errorf("step[0].Pivot = true, want false (default)")
	}

	charge := saga.Steps[1]
	if charge.Name != "charge" {
		t.Errorf("step[1].Name = %q, want charge", charge.Name)
	}

	if !charge.Pivot {
		t.Errorf("step[1].Pivot = false, want true")
	}
}

func TestParseSagaPivotFalse(t *testing.T) {
	t.Parallel()

	file := mustParseClean(t, `saga S {
		step x {
			execute: A.B
			pivot: false
		}
	}`)

	step := firstSaga(t, file).Steps[0]
	if step.Pivot {
		t.Fatalf("Pivot = true, want false")
	}
}

func TestParseSagaEmpty(t *testing.T) {
	t.Parallel()

	file := mustParseClean(t, `saga Empty {}`)

	if steps := firstSaga(t, file).Steps; len(steps) != 0 {
		t.Fatalf("steps = %d, want 0", len(steps))
	}
}

func TestParseSagaStepMissingExecute(t *testing.T) {
	t.Parallel()

	_, msgs := parseSrc(t, `saga S {
		step x {
			compensate: A.B
		}
	}`)

	assertHasMsg(t, msgs, "missing its required execute")
}

func TestParseSagaStepUnknownOption(t *testing.T) {
	t.Parallel()

	_, msgs := parseSrc(t, `saga S {
		step x {
			execute: A.B
			frobnicate: C.D
		}
	}`)

	assertHasMsg(t, msgs, "expected execute, compensate, or pivot")
}

func TestParseSagaPivotInvalidValue(t *testing.T) {
	t.Parallel()

	_, msgs := parseSrc(t, `saga S {
		step x {
			execute: A.B
			pivot: maybe
		}
	}`)

	assertHasMsg(t, msgs, "pivot must be true or false")
}

func TestParseSagaMalformedRPCRef(t *testing.T) {
	t.Parallel()

	_, msgs := parseSrc(t, `saga S {
		step x {
			execute: InventoryService
		}
	}`)

	assertHasMsg(t, msgs, `expected "."`)
}

func TestParseSagaMalformedRecovery(t *testing.T) {
	t.Parallel()

	src := `saga S {
		step first { execute: A.B }
		!!! garbage
		step second { execute: C.D }
	}

	entity After {
		id: uuid
	}`

	file, msgs := parseSrc(t, src)

	saga := firstSaga(t, file)
	if len(saga.Steps) != 2 {
		t.Fatalf("recovered steps = %d, want 2; diags: %v", len(saga.Steps), msgs)
	}

	if firstEntity(t, file).Name != "After" {
		t.Fatalf("declaration after the malformed saga was lost")
	}
}

// TestParseStrayDotStillReportsLexError guards the scope of the saga parser's
// "." handling: only a "." consumed as a Service.RPC separator retracts the
// lexer's "illegal character" diagnostic. A stray "." anywhere else must
// still be reported.
func TestParseStrayDotStillReportsLexError(t *testing.T) {
	t.Parallel()

	_, msgs := parseSrc(t, "entity X { id: uuid }\n.")

	assertHasMsg(t, msgs, "illegal character")
}

func TestParseSagaDocComment(t *testing.T) {
	t.Parallel()

	file := mustParseClean(t, `// Checkout flow across inventory and payments.
// Second line.
saga CheckoutSaga {
	step reserve { execute: A.B }
}`)

	if got := firstSaga(t, file).DocComment; got != "Checkout flow across inventory and payments.\nSecond line." {
		t.Fatalf("DocComment = %q", got)
	}
}
