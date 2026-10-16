package resolver

import (
	"errors"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/parser"
)

// TestResolvePaginatedReservedParamNames proves a paginated rpc may not
// declare a param named cursor or limit: the generated request message
// reserves both for pagination.
func TestResolvePaginatedReservedParamNames(t *testing.T) {
	t.Parallel()

	for _, name := range []string{"cursor", "limit"} {
		src := `entity Task {
	id: uuid @primary
}

service TaskService {
	rpc ListTasks(` + name + `: string) -> Task {
		http: GET "/tasks"
		paginated: true
		auth: required
	}
}`

		file, pd := parser.New("test.zen", []byte(src)).ParseFile()
		if pd.HasErrors() {
			t.Fatalf("parse errors: %v", pd)
		}

		_, diags := Resolve([]*ast.File{file})
		if !diags.HasErrors() {
			t.Fatalf("param %q on paginated rpc: want diagnostic, got none", name)
		}

		if !errors.Is(diags, ErrInvalidOption) {
			t.Errorf("param %q: want ErrInvalidOption, got %v", name, diags)
		}
	}
}

// TestResolveNonPaginatedMayUseCursorParam proves the reservation applies
// only to paginated rpcs.
func TestResolveNonPaginatedMayUseCursorParam(t *testing.T) {
	t.Parallel()

	_, _ = resolveSrc(t, `entity Task {
	id: uuid @primary
}

service TaskService {
	rpc GetTask(cursor: string) -> Task {
		http: GET "/tasks"
		auth: required
	}
}`)
}

// TestResolveHTTPPathRejectsUnsafe proves http paths with control characters,
// spaces or no leading slash are rejected before they reach any backend.
func TestResolveHTTPPathRejectsUnsafe(t *testing.T) {
	t.Parallel()

	for _, path := range []string{`/a\nfunc evil() {}`, `/a b`, `a/b`, "/a\\tb", "/café"} {
		src := "entity Task {\n\tid: uuid @primary\n}\n\nservice TaskService {\n\trpc GetTask() -> Task {\n\t\thttp: GET \"" + path + "\"\n\t\tauth: required\n\t}\n}"

		file, pd := parser.New("test.zen", []byte(src)).ParseFile()
		if pd.HasErrors() {
			t.Fatalf("path %q: parse errors: %v", path, pd)
		}

		_, diags := Resolve([]*ast.File{file})
		if !errors.Is(diags, ErrInvalidOption) {
			t.Errorf("path %q: want ErrInvalidOption, got %v", path, diags)
		}
	}
}
