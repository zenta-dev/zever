package parser

import (
	"net/http"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/ast"
	"github.com/zenta-dev/zever/internal/dsl/diag"
)

// This file closes the statement-coverage gaps left by the main suite,
// steering crafted sources through the public ParseFile entry only.
//
// Provably unreachable via the public API (documented, skipped):
//   - parseArg parser_attribute.go:112-113 (expectIdentText after the
//     isIdentLike+COLON lookahead): cur is ident-like, so expectIdentText
//     cannot fail.
//   - parseArg parser_attribute.go:117-119 (expect COLON after peek==COLON):
//     the ident consume lands cur exactly on the peeked COLON, so the
//     expect cannot fail.
//   - parseFieldDecl parser_entity.go:115-117 (expectIdentText): the caller
//     only dispatches on IDENT/ENUM/MESSAGE, all ident-like, so it cannot
//     fail.
func parseGap(t *testing.T, src string) (*ast.File, diag.List) {
	t.Helper()

	p := New("test.zen", []byte(src))
	file, errs := p.ParseFile()
	if file == nil {
		t.Fatalf("ParseFile returned nil *ast.File for input:\n%s", src)
	}

	return file, errs
}

func requireParseDiag(t *testing.T, errs diag.List, wantSub string) diag.List {
	t.Helper()

	if len(errs) == 0 {
		t.Fatalf("expected at least one diagnostic containing %q, got none", wantSub)
	}

	var out diag.List

	for _, d := range errs {
		if d.Phase != "parse" {
			t.Fatalf("diagnostic phase = %q, want %q (%v)", d.Phase, "parse", d)
		}

		if strings.Contains(d.Msg, wantSub) {
			out = append(out, d)
		}
	}

	if len(out) == 0 {
		msgs := make([]string, len(errs))
		for i, d := range errs {
			msgs[i] = d.Msg
		}

		t.Fatalf("no diagnostic containing %q, got %v", wantSub, msgs)
	}

	return out
}

func firstFieldAttr(t *testing.T, file *ast.File) *ast.Attribute {
	t.Helper()

	e := firstEntity(t, file)
	if len(e.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(e.Fields))
	}

	if len(e.Fields[0].Attributes) != 1 {
		t.Fatalf("expected 1 attribute, got %+v", e.Fields[0].Attributes)
	}

	return e.Fields[0].Attributes[0]
}

func TestParseGapFloatAndSetValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		check func(t *testing.T, file *ast.File)
	}{
		{
			name: "positive float",
			src:  `entity E { f: string @a(1.5) }`,
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				fl, ok := firstFieldAttr(t, file).Args[0].Value.(*ast.FloatLit)
				if !ok || fl.Value != 1.5 {
					t.Fatalf("value = %+v, want FloatLit{1.5}", firstFieldAttr(t, file).Args[0].Value)
				}
			},
		},
		{
			name: "negative float",
			src:  `entity E { f: string @a(-2.5) }`,
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				fl, ok := firstFieldAttr(t, file).Args[0].Value.(*ast.FloatLit)
				if !ok || fl.Value != -2.5 {
					t.Fatalf("value = %+v, want FloatLit{-2.5}", firstFieldAttr(t, file).Args[0].Value)
				}
			},
		},
		{
			name: "named float arg",
			src:  `entity E { f: string @a(ratio: 2.25) }`,
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				arg := firstFieldAttr(t, file).Args[0]
				if arg.Name != "ratio" {
					t.Fatalf("arg name = %q, want ratio", arg.Name)
				}

				fl, ok := arg.Value.(*ast.FloatLit)
				if !ok || fl.Value != 2.25 {
					t.Fatalf("value = %+v, want FloatLit{2.25}", arg.Value)
				}
			},
		},
		{
			name: "empty set",
			src:  `entity E { f: string @a({}) }`,
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				set, ok := firstFieldAttr(t, file).Args[0].Value.(*ast.SetLit)
				if !ok {
					t.Fatalf("value = %T, want *ast.SetLit", firstFieldAttr(t, file).Args[0].Value)
				}

				if len(set.Items) != 0 {
					t.Fatalf("set items = %v, want empty", set.Items)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, msgs := parseSrc(t, tt.src)
			if len(msgs) != 0 {
				t.Fatalf("expected zero diagnostics, got %v", msgs)
			}

			tt.check(t, file)
		})
	}
}

func TestParseGapNumericLiteralErrors(t *testing.T) {
	t.Parallel()

	bigFloat := strings.Repeat("9", 400) + "." + strings.Repeat("9", 100)

	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{name: "int overflow", src: `entity E { f: string @a(99999999999999999999999) }`, wantSub: "out of range"},
		{name: "float overflow", src: `entity E { f: string @a(` + bigFloat + `) }`, wantSub: "float literal"},
		{name: "set missing rbrace", src: `entity E { f: string @a({x) }`, wantSub: "expected"},
		{name: "call missing rparen", src: `entity E { f: string @a(foo(1} }`, wantSub: "expected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			e := firstEntity(t, file)
			if len(e.Fields) != 0 {
				t.Fatalf("expected malformed field to be dropped, got %d fields", len(e.Fields))
			}
		})
	}
}

func TestParseGapInvalidDurationKeptWithDiag(t *testing.T) {
	t.Parallel()

	src := `entity E { f: string @a(9999999999999999999s) }`

	file, errs := parseGap(t, src)
	found := requireParseDiag(t, errs, "invalid duration")

	if found[0].Pos.File != "test.zen" || found[0].Pos.Line != 1 {
		t.Fatalf("diag pos = %v, want test.zen:1:?", found[0].Pos)
	}

	dl, ok := firstFieldAttr(t, file).Args[0].Value.(*ast.DurationLit)
	if !ok {
		t.Fatalf("value = %T, want *ast.DurationLit (kept despite invalid)", firstFieldAttr(t, file).Args[0].Value)
	}

	if dl.Valid {
		t.Fatalf("expected Valid=false for overflowing duration, got %+v", dl)
	}

	if dl.Raw != "9999999999999999999s" {
		t.Fatalf("raw = %q, want overflowing literal", dl.Raw)
	}
}

func TestParseGapColonValueMissingColon(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
		check   func(t *testing.T, file *ast.File)
	}{
		{
			name:    "dispatch without colon",
			src:     "schedule S {\n\tdispatch Foo\n}",
			wantSub: "expected",
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				sched, ok := file.Decls[0].(*ast.ScheduleDecl)
				if !ok {
					t.Fatalf("decls[0] = %T, want *ast.ScheduleDecl", file.Decls[0])
				}

				if sched.Dispatch != nil {
					t.Fatalf("expected Dispatch to be unset, got %+v", sched.Dispatch)
				}
			},
		},
		{
			name:    "auth without colon",
			src:     "service S {\n\trpc F() -> T {\n\t\tauth required\n\t}\n}",
			wantSub: "expected",
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				svc, ok := file.Decls[0].(*ast.ServiceDecl)
				if !ok {
					t.Fatalf("decls[0] = %T, want *ast.ServiceDecl", file.Decls[0])
				}

				if svc.RPCs[0].Auth != nil {
					t.Fatalf("expected Auth to be unset, got %+v", svc.RPCs[0].Auth)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			matched := requireParseDiag(t, errs, tt.wantSub)

			if matched[0].Pos.Line != 2 && matched[0].Pos.Line != 3 {
				t.Fatalf("diag pos = %v, want the option line", matched[0].Pos)
			}

			tt.check(t, file)
		})
	}
}

func TestParseGapEnumValueErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
	}{
		{name: "int value", src: `enum E { 123 }`},
		{name: "string value", src: `enum E { "s" }`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, "expected enum value")

			if len(file.Decls) != 0 {
				t.Fatalf("expected malformed enum to be dropped, got %d decls", len(file.Decls))
			}
		})
	}
}

func TestParseGapJobDeclErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		src        string
		wantSub    string
		wantDecls  int
		wantQueue  string
		wantNoJobs bool
	}{
		{name: "missing rparen", src: `job J(a: string { queue: q }`, wantSub: "expected", wantNoJobs: true},
		{name: "missing lbrace", src: "job J()\nqueue: q", wantSub: "expected", wantNoJobs: true},
		{name: "non ident option", src: `job J() { 123 }`, wantSub: "expected queue or retry option", wantDecls: 1},
		{name: "queue missing colon", src: `job J() { queue foo }`, wantSub: "expected", wantDecls: 1, wantQueue: ""},
		{name: "retry missing colon", src: `job J() { retry foo }`, wantSub: "expected", wantDecls: 1},
		{name: "unknown option", src: `job J() { bogus: 1 }`, wantSub: "expected queue or retry option", wantDecls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			if tt.wantNoJobs {
				for _, d := range file.Decls {
					if _, ok := d.(*ast.JobDecl); ok {
						t.Fatalf("expected job decl to be dropped, got %+v", d)
					}
				}

				return
			}

			if len(file.Decls) != tt.wantDecls {
				t.Fatalf("decls = %d, want %d", len(file.Decls), tt.wantDecls)
			}

			job, ok := file.Decls[0].(*ast.JobDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.JobDecl", file.Decls[0])
			}

			if job.Queue != tt.wantQueue {
				t.Fatalf("queue = %q, want %q", job.Queue, tt.wantQueue)
			}
		})
	}
}

func TestParseGapJobOptionRecovery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		src       string
		wantQueue string
		wantRetry int
	}{
		{
			name:      "garbage then queue",
			src:       `job J() { 123 bogus queue: q }`,
			wantQueue: "q",
		},
		{
			name:      "garbage then retry",
			src:       `job J() { 123 bogus retry: foo }`,
			wantRetry: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, "expected queue or retry option")

			job, ok := file.Decls[0].(*ast.JobDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.JobDecl", file.Decls[0])
			}

			if job.Queue != tt.wantQueue {
				t.Fatalf("queue = %q, want %q", job.Queue, tt.wantQueue)
			}

			if len(job.Retry) != tt.wantRetry {
				t.Fatalf("retry len = %d, want %d", len(job.Retry), tt.wantRetry)
			}
		})
	}
}

func TestParseGapScheduleOptionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{name: "non ident option", src: `schedule S { 123 }`, wantSub: "expected cron or dispatch option"},
		{name: "unknown option", src: `schedule S { bogus: 1 }`, wantSub: "expected cron or dispatch option"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			if _, ok := file.Decls[0].(*ast.ScheduleDecl); !ok {
				t.Fatalf("decls[0] = %T, want *ast.ScheduleDecl", file.Decls[0])
			}
		})
	}
}

func TestParseGapScheduleOptionRecovery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		src   string
		check func(t *testing.T, sched *ast.ScheduleDecl)
	}{
		{
			name: "garbage then cron",
			src:  "schedule S {\n123 bogus\ncron: \"0 0 * * *\"\n}",
			check: func(t *testing.T, sched *ast.ScheduleDecl) {
				t.Helper()

				if sched.Cron != "0 0 * * *" {
					t.Fatalf("cron = %q, want spec", sched.Cron)
				}
			},
		},
		{
			name: "garbage then dispatch",
			src:  "schedule S {\n123 bogus\ndispatch: Foo\n}",
			check: func(t *testing.T, sched *ast.ScheduleDecl) {
				t.Helper()

				ident, ok := sched.Dispatch.(*ast.IdentValue)
				if !ok || ident.Name != "Foo" {
					t.Fatalf("dispatch = %+v, want IdentValue{Foo}", sched.Dispatch)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			matched := requireParseDiag(t, errs, "expected cron or dispatch option")

			if matched[0].Pos.Line != 2 {
				t.Fatalf("diag pos = %v, want line 2 (garbage line)", matched[0].Pos)
			}

			sched, ok := file.Decls[0].(*ast.ScheduleDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.ScheduleDecl", file.Decls[0])
			}

			tt.check(t, sched)
		})
	}
}

func TestParseGapServiceDeclRecovery(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{
			name:    "garbage member then rpc",
			src:     "service S {\nbogus\nrpc Ok() -> T {\nhttp: GET \"/x\"\n}\n}",
			wantSub: "expected rpc declaration",
		},
		{
			name:    "failing rpc then rpc",
			src:     "service S {\nrpc 123\nrpc Ok() -> T {\nhttp: GET \"/x\"\n}\n}",
			wantSub: "expected identifier",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			svc, ok := file.Decls[0].(*ast.ServiceDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.ServiceDecl", file.Decls[0])
			}

			if len(svc.RPCs) != 1 || svc.RPCs[0].Name != "Ok" {
				t.Fatalf("rpcs = %+v, want [Ok]", svc.RPCs)
			}

			if svc.RPCs[0].HTTP == nil || svc.RPCs[0].HTTP.Method != http.MethodGet {
				t.Fatalf("http = %+v, want http.MethodGet", svc.RPCs[0].HTTP)
			}
		})
	}
}

func TestParseGapRPCDeclErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{name: "missing rparen", src: `service S { rpc F(a: string -> T {} }`, wantSub: "expected"},
		{name: "missing lbrace", src: `service S { rpc F() -> T }`, wantSub: "expected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			svc, ok := file.Decls[0].(*ast.ServiceDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.ServiceDecl", file.Decls[0])
			}

			if len(svc.RPCs) != 0 {
				t.Fatalf("expected failing rpc to be dropped, got %+v", svc.RPCs)
			}
		})
	}
}

func TestParseGapParamErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{name: "missing colon", src: `service S { rpc F(a string) -> T {} }`, wantSub: "expected"},
		{name: "bad type", src: `service S { rpc F(a: 123) -> T {} }`, wantSub: "expected identifier"},
		{name: "bad attribute", src: `service S { rpc F(a: string @) -> T {} }`, wantSub: "expected identifier"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			svc, ok := file.Decls[0].(*ast.ServiceDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.ServiceDecl", file.Decls[0])
			}

			if len(svc.RPCs) != 0 {
				t.Fatalf("expected rpc with bad param to be dropped, got %+v", svc.RPCs)
			}
		})
	}
}

func TestParseGapTypeExprErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{name: "non ident arg", src: `entity E { f: enum(123) }`, wantSub: "expected identifier"},
		{name: "missing rparen", src: `entity E { f: enum(a }`, wantSub: "expected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			e := firstEntity(t, file)
			if len(e.Fields) != 0 {
				t.Fatalf("expected field with bad type to be dropped, got %d fields", len(e.Fields))
			}
		})
	}
}

func TestParseGapRelationIndexErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
		check   func(t *testing.T, file *ast.File)
	}{
		{
			name:    "relation bad attribute",
			src:     `entity E { has_many x: Y @ }`,
			wantSub: "expected identifier",
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				if len(firstEntity(t, file).Relations) != 0 {
					t.Fatalf("expected bad relation to be dropped")
				}
			},
		},
		{
			name:    "index missing rparen",
			src:     `entity E { index(a `,
			wantSub: "expected",
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				if len(file.Decls) != 0 {
					t.Fatalf("expected truncated entity to be dropped, got %d decls", len(file.Decls))
				}
			},
		},
		{
			name:    "index bad attribute",
			src:     `entity E { index(a) @ }`,
			wantSub: "expected identifier",
			check: func(t *testing.T, file *ast.File) {
				t.Helper()

				if len(firstEntity(t, file).Indexes) != 0 {
					t.Fatalf("expected bad index to be dropped")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)
			tt.check(t, file)
		})
	}
}

func TestParseGapJoinBlockErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
	}{
		{name: "non ident table", src: `entity E { has_many x: Y { join_table: 123 } }`, wantSub: "expected identifier"},
		{name: "missing rbrace", src: `entity E { has_many x: Y { join_table: foo `, wantSub: "expected"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			for _, d := range file.Decls {
				if e, ok := d.(*ast.EntityDecl); ok && len(e.Relations) != 0 {
					t.Fatalf("expected relation with bad join to be dropped, got %+v", e.Relations)
				}
			}
		})
	}
}

func TestParseGapRPCOptionErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantSub string
		check   func(t *testing.T, rpc *ast.RPCDecl)
	}{
		{
			name:    "unknown option",
			src:     `service S { rpc F() -> T { bogus: 1 } }`,
			wantSub: "expected http, auth, permission, errors, or paginated option",
			check: func(t *testing.T, rpc *ast.RPCDecl) {
				t.Helper()

				if rpc.HTTP != nil || rpc.Auth != nil || rpc.PaginatedSet {
					t.Fatalf("expected no options set, got %+v", rpc)
				}
			},
		},
		{
			name:    "paginated missing colon",
			src:     `service S { rpc F() -> T { paginated true } }`,
			wantSub: "expected",
			check: func(t *testing.T, rpc *ast.RPCDecl) {
				t.Helper()

				if rpc.PaginatedSet {
					t.Fatalf("expected PaginatedSet=false, got %+v", rpc)
				}
			},
		},
		{
			name:    "paginated non bool",
			src:     `service S { rpc F() -> T { paginated: foo } }`,
			wantSub: "expected true or false",
			check: func(t *testing.T, rpc *ast.RPCDecl) {
				t.Helper()

				if rpc.PaginatedSet {
					t.Fatalf("expected PaginatedSet=false, got %+v", rpc)
				}
			},
		},
		{
			name:    "errors missing colon",
			src:     `service S { rpc F() -> T { errors foo } }`,
			wantSub: "expected",
			check: func(t *testing.T, rpc *ast.RPCDecl) {
				t.Helper()

				if len(rpc.Errors) != 0 {
					t.Fatalf("expected no errors, got %+v", rpc.Errors)
				}
			},
		},
		{
			name:    "errors missing lbrace",
			src:     `service S { rpc F() -> T { errors: foo } }`,
			wantSub: "expected",
			check: func(t *testing.T, rpc *ast.RPCDecl) {
				t.Helper()

				if len(rpc.Errors) != 0 {
					t.Fatalf("expected no errors, got %+v", rpc.Errors)
				}
			},
		},
		{
			name:    "errors missing rbrace",
			src:     `service S { rpc F() -> T { errors: { Foo 123 } } }`,
			wantSub: "expected",
			check: func(t *testing.T, rpc *ast.RPCDecl) {
				t.Helper()

				if len(rpc.Errors) != 0 {
					t.Fatalf("expected malformed errors set to be dropped, got %+v", rpc.Errors)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			file, errs := parseGap(t, tt.src)
			_ = requireParseDiag(t, errs, tt.wantSub)

			svc, ok := file.Decls[0].(*ast.ServiceDecl)
			if !ok {
				t.Fatalf("decls[0] = %T, want *ast.ServiceDecl", file.Decls[0])
			}

			if len(svc.RPCs) != 1 {
				t.Fatalf("expected rpc to survive bad option, got %+v", svc.RPCs)
			}

			tt.check(t, svc.RPCs[0])
		})
	}
}

func TestParseGapRPCOptionRecovery(t *testing.T) {
	t.Parallel()

	src := "service S {\nrpc F() -> T {\nbogus: 1\nhttp: GET \"/x\"\n}\n}"

	file, errs := parseGap(t, src)
	matched := requireParseDiag(t, errs, "expected http, auth, permission, errors, or paginated option")

	if matched[0].Pos.Line != 3 {
		t.Fatalf("diag pos = %v, want line 3 (bogus line)", matched[0].Pos)
	}

	svc, ok := file.Decls[0].(*ast.ServiceDecl)
	if !ok {
		t.Fatalf("decls[0] = %T, want *ast.ServiceDecl", file.Decls[0])
	}

	if len(svc.RPCs) != 1 || svc.RPCs[0].HTTP == nil {
		t.Fatalf("expected http option to survive preceding garbage, got %+v", svc.RPCs)
	}

	if svc.RPCs[0].HTTP.Method != http.MethodGet || svc.RPCs[0].HTTP.Path != "/x" {
		t.Fatalf("http = %+v, want http.MethodGet /x", svc.RPCs[0].HTTP)
	}
}
