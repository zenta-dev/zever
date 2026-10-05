package resolver

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/dsl/ast"
	"github.com/zenta-dev/zever/dsl/parser"
)

// BenchmarkResolveAppFixture resolves the canonical compile/testdata/app.zen
// fixture: two entities with a bidirectional relation, two services with
// four RPCs, exercising every resolver pass including the hard opinions.
func BenchmarkResolveAppFixture(b *testing.B) {
	src, err := os.ReadFile("../compile/testdata/app.zen")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}

	file, diags := parser.New("app.zen", src).ParseFile()
	if diags.HasErrors() {
		b.Fatalf("parse fixture: %v", diags)
	}

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		if _, diags := Resolve([]*ast.File{file}); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}

// BenchmarkResolveAppFixtureParallel is the concurrent variant: Resolve is a
// pure function of its input files, so parallel calls are safe.
func BenchmarkResolveAppFixtureParallel(b *testing.B) {
	src, err := os.ReadFile("../compile/testdata/app.zen")
	if err != nil {
		b.Fatalf("read fixture: %v", err)
	}

	file, diags := parser.New("app.zen", src).ParseFile()
	if diags.HasErrors() {
		b.Fatalf("parse fixture: %v", diags)
	}

	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, diags := Resolve([]*ast.File{file}); diags.HasErrors() {
				b.Fatal(diags)
			}
		}
	})
}

// BenchmarkResolveEmpty is the boundary case: no files at all.
func BenchmarkResolveEmpty(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, diags := Resolve(nil); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}

// BenchmarkResolveInvalid measures the best-effort path: a schema full of
// independent errors must still run every pass over every declaration.
func BenchmarkResolveInvalid(b *testing.B) {
	src := `entity A { id: nosuchtype }
entity B { id: uuid
ref: Missing }
service S { rpc G() -> AlsoMissing { http: GET "/g" } }
job J() { queue: default }
schedule Nightly { cron: "not a cron" dispatch: Nope }
enum E { a, b }`

	file, diags := parser.New("bad.zen", []byte(src)).ParseFile()
	if diags.HasErrors() {
		b.Fatalf("parse: %v", diags)
	}

	b.ReportAllocs()
	for b.Loop() {
		_, diags := Resolve([]*ast.File{file})
		if !diags.HasErrors() {
			b.Fatal("Resolve(invalid) produced no diagnostics, want best-effort errors")
		}
	}
}

// BenchmarkResolveManyEntities measures scaling across a wide schema: 100
// entities each with a relation to the next, so relation resolution does real
// work per entity.
func BenchmarkResolveManyEntities(b *testing.B) {
	const n = 100

	var src strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&src, "entity E%d {\n  id: uuid @primary\n  name: string\n  has_many next: E%d @foreign_key(id)\n}\n\n", i, (i+1)%n)
	}

	file, diags := parser.New("many.zen", []byte(src.String())).ParseFile()
	if diags.HasErrors() {
		b.Fatalf("parse: %v", diags)
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, diags := Resolve([]*ast.File{file}); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}
