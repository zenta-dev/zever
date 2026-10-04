package compile

import (
	"os"
	"testing"

	"github.com/zenta-dev/zever/dsl/backend/atlas"
	"github.com/zenta-dev/zever/dsl/backend/gogen"
	"github.com/zenta-dev/zever/dsl/backend/openapi"
	"github.com/zenta-dev/zever/dsl/backend/proto"
	"github.com/zenta-dev/zever/dsl/backend/zenorm"
)

// loadAppFixture reads the canonical app.zen fixture.
func loadAppFixture(tb testing.TB) map[string]string {
	tb.Helper()

	src, err := os.ReadFile("testdata/app.zen")
	if err != nil {
		tb.Fatalf("read fixture: %v", err)
	}

	return map[string]string{"schema/app/app.zen": string(src)}
}

// BenchmarkCompileAppFixture runs the full pipeline (parse + resolve) over
// the canonical fixture with no backends.
func BenchmarkCompileAppFixture(b *testing.B) {
	files := loadAppFixture(b)

	b.ReportAllocs()
	for b.Loop() {
		if _, diags := Compile(files); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}

// BenchmarkCompileAppFixtureProto adds the proto backend, exercising the
// backend dispatch and output-map assembly on top of the pipeline.
func BenchmarkCompileAppFixtureProto(b *testing.B) {
	files := loadAppFixture(b)

	b.ReportAllocs()
	for b.Loop() {
		if _, diags := Compile(files, proto.New()); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}

// BenchmarkCompileAppFixtureAllPureBackends runs every pure (non-subprocess)
// backend over one Compile call: proto, gogen, atlas, openapi, zenorm.
func BenchmarkCompileAppFixtureAllPureBackends(b *testing.B) {
	files := loadAppFixture(b)

	b.ReportAllocs()
	for b.Loop() {
		if _, diags := Compile(files, proto.New(), gogen.New(), atlas.New(), openapi.New(), zenorm.New()); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}

// BenchmarkCompileEmpty is the boundary case: no files at all.
func BenchmarkCompileEmpty(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, diags := Compile(nil); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}

// BenchmarkCompileInvalid measures the error path: every file fails
// resolution, so backends must be skipped entirely.
func BenchmarkCompileInvalid(b *testing.B) {
	files := map[string]string{
		"a.zen": "entity A {\n  id: nosuchtype\n}",
		"b.zen": "entity B {\n  ref: Missing\n}",
	}

	b.ReportAllocs()
	for b.Loop() {
		if _, diags := Compile(files); diags.HasErrors() {
			b.Fatal(diags)
		}
	}
}
