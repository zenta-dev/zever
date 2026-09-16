package orm

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/db"
)

// recordingExec is a db.DB that captures every query/args pair instead of
// executing anything. It is the harness for proving the public API renders a
// FIXED SQL template for arbitrary payloads, with the payload only ever
// appearing among the bound arguments -- no database required.
type recordingExec struct {
	dialectName string
	queries     []string
	argsList    [][]any
}

func (r *recordingExec) Query(_ context.Context, query string, args ...any) (db.Rows, error) {
	r.queries = append(r.queries, query)
	r.argsList = append(r.argsList, args)

	return emptyRows{}, nil
}

func (r *recordingExec) Exec(_ context.Context, query string, args ...any) (int64, error) {
	r.queries = append(r.queries, query)
	r.argsList = append(r.argsList, args)

	return 0, nil
}

func (r *recordingExec) Ping(context.Context) error  { return nil }
func (r *recordingExec) Close(context.Context) error { return nil }
func (r *recordingExec) Dialect() string             { return r.dialectName }

// last returns the most recently captured query and bound args.
func (r *recordingExec) last() (string, []any) {
	n := len(r.queries)

	return r.queries[n-1], r.argsList[n-1]
}

// injectionOp is one fluent-API operation driven through the corpus. Every
// payload must render the byte-identical template for that op/dialect, and
// surface as the wantArg value(s) among the bound args -- never in the SQL
// text.
type injectionOp struct {
	name string
	// template returns the EXACT SQL the op must render for dial
	// ("sqlite"/"postgres"), with ph(n) the n-th placeholder marker. The
	// template is constant across payloads by construction.
	template func(dial string, ph func(n int) string) string
	// wantArg returns the value(s) the payload is expected to appear as
	// among the bound arguments (Contains escapes it; everything else binds
	// it raw).
	wantArg func(payload string) []string
	// run executes the op against exec, leaving the rendered query/args
	// captured on it.
	run func(ctx context.Context, exec *recordingExec, payload string)
}

// ph returns the placeholder writer for a dialect name.
func ph(dial string) func(n int) string {
	if dial == "postgres" {
		return func(n int) string { return "$" + strconv.Itoa(n) }
	}

	return func(int) string { return "?" }
}

// securityOps is the battery of SELECT-path public-API operations the
// corpus and fuzzer drive payloads through. (Mutation builders live with
// the mutation surface and are covered there.)
func securityOps() []injectionOp {
	selectHead := `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE `

	return []injectionOp{
		{
			name: "select-Eq",
			template: func(_ string, ph func(n int) string) string {
				return selectHead + `"name" = ` + ph(1)
			},
			wantArg: func(p string) []string { return []string{p} },
			run: func(ctx context.Context, exec *recordingExec, p string) {
				_, _ = From(widgets).Where(widgetName.Eq(p)).All(ctx, exec)
			},
		},
		{
			name: "select-In",
			template: func(_ string, ph func(n int) string) string {
				return selectHead + `"name" IN (` + ph(1) + `, ` + ph(2) + `)`
			},
			wantArg: func(p string) []string { return []string{p} },
			run: func(ctx context.Context, exec *recordingExec, p string) {
				_, _ = From(widgets).Where(widgetName.In(p, "w2")).All(ctx, exec)
			},
		},
		{
			name: "select-Like-Contains",
			template: func(_ string, ph func(n int) string) string {
				return selectHead + `"name" LIKE ` + ph(1) + ` ESCAPE '\'`
			},
			wantArg: func(p string) []string { return []string{"%" + escapeLike(p) + "%"} },
			run: func(ctx context.Context, exec *recordingExec, p string) {
				_, _ = From(widgets).Where(Contains(widgetName, p)).All(ctx, exec)
			},
		},
		{
			name: "select-Between",
			template: func(_ string, ph func(n int) string) string {
				return selectHead + `"name" BETWEEN ` + ph(1) + ` AND ` + ph(2)
			},
			wantArg: func(p string) []string { return []string{p} },
			run: func(ctx context.Context, exec *recordingExec, p string) {
				_, _ = From(widgets).Where(widgetName.Between(p, "zzz")).All(ctx, exec)
			},
		},
		{
			name: "select-UnsafeRaw-bound-arg",
			template: func(_ string, ph func(n int) string) string {
				return selectHead + `quantity > ` + ph(1)
			},
			wantArg: func(p string) []string { return []string{p} },
			run: func(ctx context.Context, exec *recordingExec, p string) {
				//lint:allow-unsafesql escape-hatch test: the payload is the bound ARG, the fragment is the fixed template
				_, _ = From(widgets).Where(UnsafeRaw[widget]("quantity > ?", p)).All(ctx, exec)
			},
		},
		{
			name: "select-JSON-extract-value",
			template: func(dial string, ph func(n int) string) string {
				if dial == "sqlite" {
					return selectHead + `json_extract("data", '$.k') = ` + ph(1)
				}

				return selectHead + `"data"->>'k' = ` + ph(1)
			},
			wantArg: func(p string) []string { return []string{p} },
			run: func(ctx context.Context, exec *recordingExec, p string) {
				jsonEq := NewJSONPredicate[widget]("widgets", "data", JSONExpr{Op: JSONExtractText, Steps: []JSONStep{JSONKey("k")}}, Eq, p)

				_, _ = From(widgets).Where(jsonEq).All(ctx, exec)
			},
		},
		{
			name: "select-FTS-match",
			template: func(dial string, ph func(n int) string) string {
				if dial == "sqlite" {
					return selectHead + `"bio" MATCH ` + ph(1)
				}

				return selectHead + `to_tsvector('english', "bio") @@ plainto_tsquery('english', ` + ph(1) + `)`
			},
			wantArg: func(p string) []string { return []string{p} },
			run: func(ctx context.Context, exec *recordingExec, p string) {
				fts := NewFTSPredicate[widget]("widgets", "bio", FTSExpr{Op: FTSMatch, Query: p})

				_, _ = From(widgets).Where(fts).All(ctx, exec)
			},
		},
	}
}

// argContains reports whether args contains want at any position.
func argContains(args []any, want string) bool {
	for _, a := range args {
		if s, ok := a.(string); ok && s == want {
			return true
		}
	}

	return false
}

// loadInjectionCorpus reads every non-empty, non-comment line of every file
// under testdata/injection-corpus/ as one injection payload.
func loadInjectionCorpus(tb testing.TB) []string {
	tb.Helper()

	files, err := filepath.Glob(filepath.Join("testdata", "injection-corpus", "*.txt"))
	if err != nil {
		tb.Fatalf("glob injection corpus: %v", err)
	}

	var payloads []string

	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			tb.Fatalf("open %s: %v", f, err)
		}

		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)

		for sc.Scan() {
			line := sc.Text()
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			payloads = append(payloads, line)
		}

		if err := sc.Err(); err != nil {
			tb.Fatalf("scan %s: %v", f, err)
		}

		_ = fh.Close()
	}

	return payloads
}

// TestPublicAPIInjectionCorpus drives the full corpus through every fluent
// operation on both dialects, asserting the rendered SQL is byte-for-byte
// the fixed template (so no payload can be concatenated into SQL text) and
// the payload is present among the placeholder-bound args.
func TestPublicAPIInjectionCorpus(t *testing.T) {
	ctx := context.Background()
	payloads := loadInjectionCorpus(t)
	ops := securityOps()

	if len(payloads) == 0 {
		t.Fatal("injection corpus is empty")
	}

	for _, dial := range []string{"sqlite", "postgres"} {
		for i, payload := range payloads {
			if payload == "" {
				continue
			}

			t.Run(dial+"-"+strconv.Itoa(i), func(t *testing.T) {
				for _, op := range ops {
					exec := &recordingExec{dialectName: dial}
					op.run(ctx, exec, payload)

					sqlText, args := exec.last()

					if want := op.template(dial, ph(dial)); sqlText != want {
						t.Errorf("%s: SQL text = %q, want fixed template %q (payload %q must never reach SQL text)", op.name, sqlText, want, payload)
					}

					for _, want := range op.wantArg(payload) {
						if !argContains(args, want) {
							t.Errorf("%s: bound args %#v missing value %q (payload must be placeholder-bound)", op.name, args, want)
						}
					}
				}
			})
		}
	}
}

// TestUnsafeIdent proves UnsafeIdent is the codegen-only identifier escape
// hatch: an ident must appear in the caller-supplied allowlist (schema
// derived from codegen's Columns()), anything else returns an error, never a
// forged column.
func TestUnsafeIdent(t *testing.T) {
	allowlist := []string{"id", "name", "quantity", "bio"}

	t.Run("allowlisted ident accepted", func(t *testing.T) {
		//lint:allow-unsafesql escape-hatch test: ident is from the test's own allowlist
		col, err := UnsafeIdent[widget, string]("name", allowlist)
		if err != nil {
			t.Fatalf("UnsafeIdent(name, allowlist) = %v, want nil error", err)
		}

		if col.Name() != "name" {
			t.Fatalf("col.Name() = %q, want %q", col.Name(), "name")
		}

		// The returned Column renders through the normal fluent path as a
		// quoted column, never a raw string.
		exec := &recordingExec{dialectName: "sqlite"}
		_, _ = From(widgets).Where(col.Eq("v")).All(context.Background(), exec)

		if got, _ := exec.last(); got != `SELECT "id", "name", "quantity", "bio" FROM "widgets" WHERE "name" = ?` {
			t.Fatalf("UnsafeIdent column rendered SQL %q, want quoted-column Eq template", got)
		}
	})

	t.Run("non-allowlisted ident errors", func(t *testing.T) {
		//lint:allow-unsafesql escape-hatch test: asserts the non-allowlisted path returns an error
		_, err := UnsafeIdent[widget, string]("name; DROP TABLE widgets", allowlist)
		if err == nil {
			t.Fatal("UnsafeIdent with an injection ident succeeded, want an error")
		}
	})

	t.Run("empty ident errors", func(t *testing.T) {
		//lint:allow-unsafesql escape-hatch test: asserts the empty-ident path returns an error
		_, err := UnsafeIdent[widget, string]("", allowlist)
		if err == nil {
			t.Fatal("UnsafeIdent(\"\") succeeded, want an error")
		}
	})
}

// TestEncodeArgsNeverInterpolates proves bound values -- including hostile
// strings -- travel to the driver as args, never formatted into SQL text,
// on both dialects.
func TestEncodeArgsNeverInterpolates(t *testing.T) {
	ctx := context.Background()

	for _, dial := range []string{"sqlite", "postgres"} {
		exec := &recordingExec{dialectName: dial}

		payload := `x" OR "1"="1"; DROP TABLE widgets; --`

		_, _ = From(widgets).Where(widgetName.Eq(payload)).All(ctx, exec)

		q, args := exec.last()

		if strings.Contains(q, payload) {
			t.Fatalf("%s: payload reached SQL text: %q", dial, q)
		}

		if !argContains(args, payload) {
			t.Fatalf("%s: bound args %#v missing payload", dial, args)
		}
	}
}

// FuzzPublicAPINoInjection is the injection-proofing fuzz target: any
// payload driven through the fluent public API must leave the rendered SQL
// byte-identical to the fixed template, with the payload only ever among
// the bound args. It runs as an ordinary seed-corpus regression test under
// `go test ./orm/`; run with `-fuzz=FuzzPublicAPINoInjection
// -fuzztime=...` for continuous fuzzing.
func FuzzPublicAPINoInjection(f *testing.F) {
	// Seed corpus: the testdata payloads plus exotic bytes (NUL, unicode,
	// quotes, LIKE wildcards, placeholder markers) that are awkward to
	// store in a text file.
	for _, p := range loadInjectionCorpus(f) {
		f.Add(p)
	}

	for _, p := range []string{
		"' OR 1=1 --",
		"'; DROP TABLE widgets; --",
		"'\x00' OR 1=1 --\x00",
		"\x00",
		"漢字' OR 1=1 -- 🚀",
		"'",
		"''",
		"'''",
		"\\",
		"%",
		"_",
		"?",
		"$1",
		"1=1",
		"what? OR 1=1",
	} {
		f.Add(p)
	}

	ops := securityOps()

	f.Fuzz(func(t *testing.T, payload string) {
		if payload == "" {
			t.Skip("empty payload trivially matches; not an injection signal")
		}

		ctx := context.Background()

		for _, op := range ops {
			exec := &recordingExec{dialectName: "sqlite"}
			op.run(ctx, exec, payload)

			sqlText, args := exec.last()

			if want := op.template("sqlite", ph("sqlite")); sqlText != want {
				t.Fatalf("%s: SQL text = %q, want fixed template %q for payload %q", op.name, sqlText, want, payload)
			}

			for _, want := range op.wantArg(payload) {
				if !argContains(args, want) {
					t.Fatalf("%s: bound args %#v missing value %q for payload %q", op.name, args, want, payload)
				}
			}
		}
	})
}
