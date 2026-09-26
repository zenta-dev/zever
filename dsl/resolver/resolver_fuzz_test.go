package resolver

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/parser"
)

// repoFixture reads a file at rel, relative to the module root, using the
// caller's own source location to find the root regardless of the working
// directory `go test` was invoked from.
func repoFixture(f *testing.F, rel string) string {
	f.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		f.Fatalf("runtime.Caller failed while locating fixture %q", rel)
	}

	// dsl/resolver -> module root is one level up.
	root := filepath.Join(filepath.Dir(file), "..")

	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		f.Fatalf("reading fixture %q: %v", rel, err)
	}

	return string(data)
}

// FuzzResolve extends the lexer/parser "never panics on malformed input"
// design philosophy one stage further: it feeds the source string through
// the real parser (as FuzzParse does) and then runs whatever AST comes back
// -- complete, partial, or entirely empty -- through resolver.Resolve.
//
// NOTE (worth flagging, per the task brief): unlike lexer.go and parser.go,
// this package's own doc comment does NOT explicitly claim "never panics".
// It says passes "report problems as diag.Diagnostic values rather than
// failing fast", which is a related but weaker claim (it's about not
// stopping early on the happy AST-walking path, not specifically about
// surviving a malformed/partial AST without panicking). This fuzz test
// enforces the stronger never-panics property mechanically, but the
// resolver's documented design contract should probably be tightened to
// match if that's confirmed to be an intentional invariant.
func FuzzResolve(f *testing.F) {
	seeds := []string{
		"",
		"entity User {\n\t\tid: uuid\n\t}",
		`entity User {
			id: uuid @primary
			email: string @unique @validate(min_len: 1)
			created_at: timestamp @default(now())
		}

		entity Order @schema(billing) {
			id: uuid @primary
			total_cents: int64
			status: enum(pending, paid, shipped, cancelled) @default(pending)

			belongs_to user: User
			has_many items: OrderItem
			many_to_many tags: Tag {
				join_table: order_tags
			}

			index(user_id)
			index(status) @unique
		}

		service OrderService {
			rpc GetOrder(id: uuid) -> Order {
				http: GET "/orders/{id}"
				auth: required(roles: {owner, admin})
			}
		}

		job SendConfirmationEmail(order_id: uuid) {
			queue: emails
			retry: max_attempts(10), backoff(exponential, base: 30s)
		}

		schedule DailyCleanup {
			cron: "0 0 * * *"
			dispatch: CleanupAbandonedCarts()
		}`,
		// Duplicate top-level declarations.
		"entity User { id: uuid }\nentity User { id: uuid }",
		// Unresolved type reference.
		"entity User {\n\t\tid: Nonexistent\n\t}",
		// Unresolved relation target.
		"entity Order {\n\t\thas_many items: Nonexistent\n\t}",
		// Malformed/partial AST inputs (parser recovers, resolver must too).
		"entity Bad { !!! }\n\nentity Good {\n\tid: uuid\n}",
		"entity Foo {\n\t\tid: uuid",
		`service S {
			rpc Get(id: uuid) -> Thing {
				!!!
				http: GET "/things/{id}"
			}
		}`,
		"message Bad { !!! }\n\nentity Good {\n\tid: uuid\n}",
		// Named enum: valid reference, duplicate name vs. entity, unresolved
		// reference, and @default validated against a named enum's values.
		"enum Status { active, inactive }\n\nentity Task {\n\tid: uuid\n\tstatus: Status\n}",
		"enum Status { active, inactive }\nentity Status { id: uuid }",
		"entity Task {\n\tid: uuid\n\tstatus: Unknown\n}",
		"enum Status { active, inactive }\n\nentity Task {\n\tid: uuid\n\tstatus: Status @default(bogus)\n}",
		"enum Status { active, active }",
		"enum Status {}",
		// Astral-plane runes in doc comments / literals — column counting, never panic.
		"// 💖 astral\nentity User { id: uuid }",
		"entity User { name: string @default(\"😀\") }",
		// Deep nesting — resolver must handle deeply unbalanced/balanced AST without panic.
		"entity Deep { id: uuid } " + strings.Repeat("{", 256) + strings.Repeat("}", 256),
		"entity Foo { id: uuid @validate(nested(" + strings.Repeat("(", 128) + strings.Repeat(")", 128) + ")) }",
		// 10k-depth nesting — R28/R29: resolver must not panic/stack-overflow on huge balanced/unbalanced input.
		strings.Repeat("{", 10000) + strings.Repeat("}", 10000),
		"entity Deep { id: uuid } " + strings.Repeat("{", 10000) + strings.Repeat("}", 10000),
		"entity Foo { id: uuid @validate(nested(" + strings.Repeat("(", 5000) + strings.Repeat(")", 5000) + ")) }",
		strings.Repeat("😀", 500) + "\nentity User { id: uuid }",
		// Cross-module version duplicate — duplicate decl across flat name space (resolver is flat globally).
		"entity User { id: uuid }\nentity User { id: uuid }",
		// Same logical duplicate that would arise from two versioned modules v1/iam and v2/iam
		// both declaring User (flat duplicate check still fires) — seed ensures fuzz covers it.
		"entity User { id: uuid } // schema/v1/iam/user.zen duplicate of schema/v2/iam/user.zen",
		// Explicit R29 cross-module version duplicate: two files both declare same entity name.
		"entity User { id: uuid } // file: schema/v1/iam/user.zen\nentity User { id: uuid } // file: schema/v2/iam/user.zen duplicate decl",
		"entity Order { id: uuid }\nentity Order { id: uuid } // cross-module duplicate triggers flat duplicate diagnostic",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	for _, rel := range []string{
		"testdata/todo.zen",
		"testdata/demoapp.zen",
		"compile/testdata/app.zen",
	} {
		f.Add(repoFixture(f, rel))
	}

	f.Fuzz(func(t *testing.T, src string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Resolve panicked on input %q: %v", src, r)
			}
		}()

		file, _ := parser.New("fuzz.zen", []byte(src)).ParseFile()

		_, _ = Resolve([]*ast.File{file})

		// Cross-module version duplicate and nil-file guard: ResolveWithSchemaDir must not panic
		// on versioned paths declaring same name (flat duplicate) or on nil/empty File entries (R24).
		p1, _ := parser.New("schema/v1/iam/user.zen", []byte(src)).ParseFile()
		p2, _ := parser.New("schema/v2/iam/user.zen", []byte(src)).ParseFile()
		_, _ = ResolveWithSchemaDir([]*ast.File{p1, p2}, "schema")
		_, _ = ResolveWithSchemaDir([]*ast.File{nil, p1, {Name: "", Decls: nil}}, "schema")
		_, _ = ResolveWithSchemaDir([]*ast.File{nil, nil}, "schema")
	})
}
