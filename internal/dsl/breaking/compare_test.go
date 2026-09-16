package breaking

import (
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/ir"
	"github.com/zenta-dev/zever/internal/dsl/parser"
	"github.com/zenta-dev/zever/internal/dsl/resolver"
)

// resolveSrc parses and resolves a single-file schema, failing the test on
// any resolver error (warnings, e.g. version-path drift, are fine).
func resolveSrc(t *testing.T, name, src string) *ir.Schema {
	t.Helper()

	f, diags := parser.New(name, []byte(src)).ParseFile()
	if diags.HasErrors() {
		t.Fatalf("parse errors in %s: %v", name, diags)
	}

	sch, rdiags := resolver.Resolve([]*ast.File{f})
	if rdiags.HasErrors() {
		t.Fatalf("resolve errors in %s: %v", name, rdiags)
	}

	return sch
}

func TestCompareEntityRemoved(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}`)
	next := resolveSrc(t, "a.zen", ``)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindEntityRemoved)
}

func TestCompareEntityAdded(t *testing.T) {
	old := resolveSrc(t, "a.zen", ``)
	next := resolveSrc(t, "b.zen", `entity User {
		id: uuid @primary
	}`)

	changes := Compare(old, next)

	assertHasNonBreaking(t, changes, KindEntityAdded)
}

func TestCompareFieldRemoved(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
		name: string
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}`)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindFieldRemoved)
}

func TestCompareFieldAdded(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
		name: string
	}`)

	changes := Compare(old, next)

	assertHasNonBreaking(t, changes, KindFieldAdded)
}

func TestCompareFieldTypeChanged(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
		age: int32
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
		age: int64
	}`)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindFieldTypeChanged)
}

func TestCompareFieldGainsValidateIsNonBreaking(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
		name: string
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
		name: string @validate(min_len: 1)
	}`)

	changes := Compare(old, next)

	assertHasNonBreaking(t, changes, KindValidateAdded)

	for _, c := range changes {
		if c.Kind == KindFieldTypeChanged {
			t.Fatalf("gaining a @validate rule must not be reported as a type change: %+v", changes)
		}
	}
}

func TestCompareMessageRemovedAndAdded(t *testing.T) {
	old := resolveSrc(t, "a.zen", `message Greeting {
		text: string
	}`)
	next := resolveSrc(t, "a.zen", ``)

	changes := Compare(old, next)
	assertHasBreaking(t, changes, KindMessageRemoved)

	changes = Compare(next, old)
	assertHasNonBreaking(t, changes, KindMessageAdded)
}

func TestCompareServiceAndOperationRemoved(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}`)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindServiceRemoved)
}

func TestCompareOperationRemoved(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
		rpc ListUsers() -> User {
			http: GET "/users"
			auth: required
		}
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
	}`)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindOperationRemoved)
}

func TestCompareOperationAdded(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
		rpc ListUsers() -> User {
			http: GET "/users"
			auth: required
		}
	}`)

	changes := Compare(old, next)

	assertHasNonBreaking(t, changes, KindOperationAdded)
}

func TestCompareOperationParamsChanged(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid, extra: string) -> User {
			http: GET "/users/{id}"
			auth: required
		}
	}`)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindOperationParamsChanged)
}

func TestCompareOperationReturnsChanged(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	message Empty {}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	message Empty {}

	service UserService {
		rpc GetUser(id: uuid) -> Empty {
			http: GET "/users/{id}"
			auth: required
		}
	}`)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindOperationReturnsChanged)
}

func TestCompareOperationHTTPChanged(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: GET "/users/{id}"
			auth: required
		}
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}

	service UserService {
		rpc GetUser(id: uuid) -> User {
			http: POST "/users/{id}"
			auth: required
		}
	}`)

	changes := Compare(old, next)

	assertHasBreaking(t, changes, KindOperationHTTPChanged)
}

func TestCompareUnrelatedModuleUntouched(t *testing.T) {
	old := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}`)
	next := resolveSrc(t, "a.zen", `entity User {
		id: uuid @primary
	}`)

	changes := Compare(old, next)

	if len(changes) != 0 {
		t.Fatalf("identical schemas should produce zero changes, got %+v", changes)
	}
}

func TestHasBreaking(t *testing.T) {
	if HasBreaking(nil) {
		t.Fatalf("HasBreaking(nil) = true, want false")
	}

	if HasBreaking([]Change{{Breaking: false}}) {
		t.Fatalf("HasBreaking with only non-breaking changes = true, want false")
	}

	if !HasBreaking([]Change{{Breaking: false}, {Breaking: true}}) {
		t.Fatalf("HasBreaking with a breaking change present = false, want true")
	}
}

func assertHasBreaking(t *testing.T, changes []Change, kind Kind) {
	t.Helper()

	for _, c := range changes {
		if c.Kind == kind {
			if !c.Breaking {
				t.Fatalf("change %q found but Breaking = false: %+v", kind, c)
			}

			return
		}
	}

	t.Fatalf("expected a %q change, got: %+v", kind, changes)
}

func assertHasNonBreaking(t *testing.T, changes []Change, kind Kind) {
	t.Helper()

	for _, c := range changes {
		if c.Kind == kind {
			if c.Breaking {
				t.Fatalf("change %q found but Breaking = true, want false: %+v", kind, c)
			}

			return
		}
	}

	t.Fatalf("expected a %q change, got: %+v", kind, changes)
}
