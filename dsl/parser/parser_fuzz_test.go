package parser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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

	// dsl/parser -> module root is one level up.
	root := filepath.Join(filepath.Dir(file), "..")

	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		f.Fatalf("reading fixture %q: %v", rel, err)
	}

	return string(data)
}

// FuzzParse proves the parser's documented "never panics, always returns a
// non-nil *ast.File" invariant (see the package doc comment) mechanically
// rather than by hand-written test case alone.
func FuzzParse(f *testing.F) {
	// Seeds extracted from parser_test.go's own hand-written positive
	// cases -- entity/service/rpc/job/schedule/module grammar, attributes,
	// relations, indexes, enums, negative numbers, errors: sets, etc.
	seeds := []string{
		"",
		"entity User {\n\t\tid: uuid\n\t}",
		"entity User {\n\t\tid: uuid\n\t\temail: string?\n\t}",
		`entity User {
			nickname: string? @default("anon")
		}`,
		`entity User {
			email: string @unique @validate(min_len: 1)
		}`,
		`entity User {
			id: uuid @bogus(1, 2, named: 3)
		}`,
		`entity User {
			created_at: timestamp @default(now())
		}`,
		`entity User {
			status: enum(active, inactive) @default(active)
		}`,
		"entity Order {\n\thas_many related: Target\n}",
		"entity Order {\n\thas_one related: Target\n}",
		"entity Order {\n\tbelongs_to related: Target\n}",
		"entity Order {\n\tmany_to_many related: Target\n}",
		`entity Order {
			many_to_many tags: Tag {
				join_table: order_tags
			}
		}`,
		`entity Order {
			user_id: uuid
			index(user_id)
			index(user_id) @unique
		}`,
		`service OrderService {
			rpc GetOrder(id: uuid) -> Order {
				http: GET "/orders/{id}"
				auth: required(roles: {owner, admin})
				permission: check(resource: order)
			}
		}`,
		`service OrderService {
			rpc GetOrder(id: uuid) -> Order {
				http: GET "/orders/{id}"
				auth: required
				permission: check(resource: order)
				errors: { not_found, invalid_argument("email is malformed") }
			}
		}`,
		`service OrderService {
			rpc GetOrder(id: uuid) -> Order {
				errors: {}
			}
		}`,
		`service TaskService {
			rpc ListTasks(user_id: uuid) -> Task {
				http: GET "/tasks"
				auth: required
				paginated: true
			}
		}`,
		`service TaskService {
			rpc GetTask(id: uuid) -> Task {
				paginated: false
			}
		}`,
		`job SendConfirmationEmail(order_id: uuid) {
			queue: emails
			retry: max_attempts(10), backoff(exponential, base: 30s)
		}`,
		`schedule DailyCleanup {
			cron: "0 0 * * *"
			dispatch: CleanupAbandonedCarts()
		}`,
		`entity Product {
			discount: int64 @default(-5)
		}`,
		`entity Order @schema(billing_archive) {
			id: uuid
		}`,
		`message CreateOrderRequest {
			user_id: uuid @validate(format: "uuid")
			total_cents: int64 @validate(gte: 0)
		}`,
		// Full worked example (TestParseWorkedExample).
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

			rpc CreateOrder(user_id: uuid, total_cents: int64) -> Order {
				http: POST "/orders"
				auth: required(roles: {owner})
				permission: check(resource: order)
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
		// Doc comments immediately above declarations and members, including
		// the multi-line and blank-line-breaks-attachment cases.
		"// doc\nentity User {\n\t// field doc\n\tid: uuid\n}",
		"// doc line 1\n// doc line 2\nentity User {\n\tid: uuid\n}",
		"// unrelated\n\nentity User {\n\tid: uuid\n}",
		"entity Other {\n\tid: uuid\n} // trailing\nentity User {\n\tid: uuid\n}",
		"// service doc\nservice S {\n\t// rpc doc\n\trpc G() -> T {\n\t}\n}",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	// Deliberately-malformed seeds, drawn from parser_recovery_test.go's own
	// recovery-guarantee cases plus a few extra adversarial shapes the plan
	// calls out by name (unclosed entity, rpc missing return type, relation
	// with no target, mismatched nested attribute calls, unbalanced
	// errors: {} set).
	malformed := []string{
		`entity Bad {
			: uuid
			has_many orders: Order
		}

		entity Good {
			id: uuid
		}`,
		"entity Bad { !!! }\n\n\tentity Good {\n\t\tid: uuid\n\t\tname: string\n\t}",
		"entity Foo {\n\t\tid: uuid", // missing closing brace, truncated at EOF
		`service S {
			rpc Get(id: uuid) -> Thing {
				!!!
				http: GET "/things/{id}"
			}
		}`,
		`service S {
			rpc Get(id: uuid) -> Thing {
				errors: { not_found,
			}
		}

		service T {
			rpc Ping(x: uuid) -> Thing {
				http: GET "/ping"
			}
		}`,
		`message Bad {
			: uuid
		}

		entity Good {
			id: uuid @primary
		}`,
		"message Bad { !!! }\n\n\tentity Good {\n\t\tid: uuid\n\t\tname: string\n\t}",
		// A rpc declaration missing its return type entirely.
		`service S { rpc Get(id: uuid) { http: GET "/x" } }`,
		// A relation with no target.
		"entity Order {\n\thas_many related:\n}",
		// Nested attribute calls with mismatched parens.
		`entity User { id: uuid @validate(nested(1, 2) }`,
		// Unclosed entity, no closing brace at all before EOF.
		"entity Foo { id: uuid",
		// Deeply unbalanced braces.
		"entity Foo {{{{{{{{{{{{{{{{{{{{",
		"}}}}}}}}}}}}}}}}}}}}",
		// Lone special characters.
		"@", "?", "@?@?@?",
		"entity 123 { }",
		"entity {}",
		// Named top-level enum declaration, well-formed and malformed.
		"enum Status { active, inactive, archived }",
		"enum Status { active, inactive, }",
		"enum Status {}",
		"enum Status { active",
		"enum { active, inactive }",
		"enum Status active, inactive }",
		"enum Status { active inactive }",
		"entity Task {\n  id: uuid @primary\n  status: Status\n}\nenum Status { active, inactive }",
		// Astral-plane runes in identifiers/comments/strings — lexer col 1-per-rune, parser never-panic.
		"// 💖 astral comment\nentity User { id: uuid }",
		"entity User { name: string @default(\"💖\") }",
		"entity User { // 😀 trailing astral\n id: uuid\n }",
		// Deep nesting — verifies syncTopLevel depth tracks braces and parens without early exit.
		"entity Deep { id: uuid } " + strings.Repeat("{{{{", 64) + strings.Repeat("}}}}", 64),
		"entity Foo { id: uuid @validate(nested(" + strings.Repeat("(", 128) + strings.Repeat(")", 128) + ")) }",
		"service S { rpc Get(" + strings.Repeat("(a: uuid, ", 32) + "x: uuid" + strings.Repeat(")", 32) + ") -> T { http: GET \"/x\" } }",
		// 10k-depth nesting — R28/R29: parser syncTopLevel must not panic/stack-overflow, always returns non-nil File.
		strings.Repeat("{", 10000) + strings.Repeat("}", 10000),
		strings.Repeat("(", 10000) + strings.Repeat(")", 10000),
		"entity Deep { id: uuid } " + strings.Repeat("{", 10000) + strings.Repeat("}", 10000),
		"entity Foo { id: uuid @validate(nested(" + strings.Repeat("(", 5000) + strings.Repeat(")", 5000) + ")) }",
		// Deep valid call-chain nesting (distinct from the bare
		// bracket-character seeds above): exercises the
		// parseValue/parseCallOrIdent/parseArgList/parseArg recursion cycle's
		// depth guard (maxValueDepth in parser_attribute.go), since a bare
		// "(" isn't a valid Value start and never reaches that path.
		"entity Foo { id: uuid @validate(" + strings.Repeat("nested(", 5000) + "1" + strings.Repeat(")", 5000) + ") }",
		// Astral emoji explicit — R28
		"😀",
		"entity User { name: string @default(\"😀\") }",
		strings.Repeat("😀", 200) + "\nentity User { id: uuid }",
		// Cross-module version duplicate decl — R29 flat duplicate still fires for v1/v2 (see resolver)
		"entity User { id: uuid }\nentity User { id: uuid } // v1/iam vs v2/iam duplicate",
		"enum Status { active }\nentity Status { id: uuid } // enum vs entity duplicate",
	}
	for _, s := range malformed {
		f.Add(s)
	}

	// Real .zen fixtures used elsewhere in the repo.
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
				t.Fatalf("ParseFile panicked on input %q: %v", src, r)
			}
		}()

		p := New("fuzz.zen", []byte(src))

		file, _ := p.ParseFile()
		if file == nil {
			t.Fatalf("ParseFile returned a nil *ast.File for input %q, want always-non-nil per package doc contract", src)
		}
	})
}
