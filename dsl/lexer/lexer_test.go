package lexer

import (
	"testing"

	"github.com/zenta-dev/zever/dsl/token"
)

// lexAll runs the lexer over src until EOF, returning every token produced,
// including the terminal EOF token.
func lexAll(t *testing.T, src string) []token.Token {
	t.Helper()

	l := New("test.zen", []byte(src))

	var toks []token.Token

	for {
		tok := l.Next()
		toks = append(toks, tok)

		if tok.Kind == token.EOF {
			return toks
		}
	}
}

func kinds(toks []token.Token) []token.Kind {
	out := make([]token.Kind, len(toks))
	for i, tok := range toks {
		out[i] = tok.Kind
	}

	return out
}

func assertKinds(t *testing.T, toks []token.Token, want []token.Kind) {
	t.Helper()

	got := kinds(toks)
	if len(got) != len(want) {
		t.Fatalf("kind count = %d, want %d; got %v want %v", len(got), len(want), got, want)
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kind[%d] = %s, want %s (all: got %v want %v)", i, got[i], want[i], got, want)
		}
	}
}

func TestKindSequences(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []token.Kind
	}{
		{
			name: "empty input",
			src:  "",
			want: []token.Kind{token.EOF},
		},
		{
			name: "entity block skeleton",
			src:  `entity User { id: uuid }`,
			want: []token.Kind{
				token.ENTITY, token.IDENT, token.LBRACE,
				token.IDENT, token.COLON, token.IDENT,
				token.RBRACE, token.EOF,
			},
		},
		{
			name: "rpc signature with arrow",
			src:  `rpc Get(id: uuid) -> User`,
			want: []token.Kind{
				token.RPC, token.IDENT, token.LPAREN, token.IDENT, token.COLON, token.IDENT,
				token.RPAREN, token.ARROW, token.IDENT, token.EOF,
			},
		},
		{
			name: "attribute at sign",
			src:  `@auth`,
			want: []token.Kind{token.AT, token.IDENT, token.EOF},
		},
		{
			name: "duration after colon and space is a fresh token, not glued",
			src:  `interval: 5m`,
			want: []token.Kind{token.IDENT, token.COLON, token.DURATION, token.EOF},
		},
		{
			name: "backoff base duration in call",
			src:  `backoff(base: 30s)`,
			want: []token.Kind{
				token.IDENT, token.LPAREN, token.IDENT, token.COLON, token.DURATION, token.RPAREN, token.EOF,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			assertKinds(t, toks, tt.want)
		})
	}
}

// TestKeywordVsIdentClassification proves the token-vs-keyword decision
// from Task 2 holds in practice: every reserved keyword lexes to its own
// Kind, while every type name, contextual block label, and HTTP verb lexes
// to plain IDENT.
func TestKeywordVsIdentClassification(t *testing.T) {
	tests := []struct {
		src  string
		want token.Kind
	}{
		// Reserved keywords.
		{"entity", token.ENTITY},
		{"service", token.SERVICE},
		{"job", token.JOB},
		{"schedule", token.SCHEDULE},
		{"rpc", token.RPC},
		{"index", token.INDEX},
		{"enum", token.ENUM},
		{"has_many", token.HAS_MANY},
		{"has_one", token.HAS_ONE},
		{"belongs_to", token.BELONGS_TO},
		{"many_to_many", token.MANY_TO_MANY},
		{"message", token.MESSAGE},
		{"true", token.TRUE},
		{"false", token.FALSE},
		// Type names -- not reserved.
		{"uuid", token.IDENT},
		{"string", token.IDENT},
		{"int64", token.IDENT},
		{"timestamp", token.IDENT},
		// Contextual block labels -- not reserved.
		{"http", token.IDENT},
		{"auth", token.IDENT},
		{"permission", token.IDENT},
		{"queue", token.IDENT},
		{"retry", token.IDENT},
		{"cron", token.IDENT},
		{"dispatch", token.IDENT},
		{"join_table", token.IDENT},
		// HTTP verbs -- not reserved.
		{"GET", token.IDENT},
		{"POST", token.IDENT},
		{"PUT", token.IDENT},
		{"DELETE", token.IDENT},
		{"PATCH", token.IDENT},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			if len(toks) != 2 {
				t.Fatalf("token count = %d, want 2 (single token + EOF); got %v", len(toks), kinds(toks))
			}

			if toks[0].Kind != tt.want {
				t.Fatalf("Kind = %s, want %s", toks[0].Kind, tt.want)
			}

			if toks[0].Lit != tt.src {
				t.Fatalf("Lit = %q, want %q", toks[0].Lit, tt.src)
			}
		})
	}
}

func TestNumbers(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []token.Kind
		lits []string
	}{
		{"plain int", "42", []token.Kind{token.INT, token.EOF}, []string{"42"}},
		{"zero", "0", []token.Kind{token.INT, token.EOF}, []string{"0"}},
		{"plain float", "3.14", []token.Kind{token.FLOAT, token.EOF}, []string{"3.14"}},
		{
			"trailing dot rejected: INT then ILLEGAL(.)",
			"1.",
			[]token.Kind{token.INT, token.ILLEGAL, token.EOF},
			[]string{"1", "."},
		},
		{
			"leading dot rejected: ILLEGAL(.) then INT",
			".5",
			[]token.Kind{token.ILLEGAL, token.INT, token.EOF},
			[]string{".", "5"},
		},
		{
			"int, stray dot, ident",
			"1.foo",
			[]token.Kind{token.INT, token.ILLEGAL, token.IDENT, token.EOF},
			[]string{"1", ".", "foo"},
		},
		{
			"float requires digit after dot -- two dots stay INT + illegal + illegal",
			"1..2",
			[]token.Kind{token.INT, token.ILLEGAL, token.ILLEGAL, token.INT, token.EOF},
			[]string{"1", ".", ".", "2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			assertKinds(t, toks, tt.want)

			for i, lit := range tt.lits {
				if toks[i].Lit != lit {
					t.Fatalf("tok[%d].Lit = %q, want %q", i, toks[i].Lit, lit)
				}
			}
		})
	}
}

func TestDurationSingleUnit(t *testing.T) {
	tests := []struct {
		name string
		src  string
		lit  string
	}{
		{"nanoseconds", "500ns", "500ns"},
		{"microseconds ascii", "10us", "10us"},
		{"microseconds mu (multi-byte rune)", "10µs", "10µs"},
		{"milliseconds", "100ms", "100ms"},
		{"seconds", "30s", "30s"},
		{"minutes", "5m", "5m"},
		{"hours", "1h", "1h"},
		{"ms wins over m (longest match first)", "5ms", "5ms"},
		{"us wins over nothing, not confused with µs", "5us", "5us"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			assertKinds(t, toks, []token.Kind{token.DURATION, token.EOF})

			if toks[0].Lit != tt.lit {
				t.Fatalf("Lit = %q, want %q", toks[0].Lit, tt.lit)
			}
		})
	}
}

// TestDurationCompoundSplit pins down the exact three-token split for a
// compound duration expression. Compound durations (Go-style "1h30m") are a
// deliberate v1 non-goal: only a single unit is ever consumed per number.
//
// "1h" scans as a fresh DURATION: the leading "1" starts a brand new token
// (nothing but source-start precedes it). The following "30" is scanned as
// a bare INT and is NOT re-checked for a duration suffix, because its first
// digit is glued directly (no separating whitespace) onto the tail of the
// just-emitted DURATION's unit letter "h" -- that adjacency is the signal
// that this is a concatenated/compound sequence rather than a fresh
// literal, so duration-suffix matching is skipped for it. The trailing "m"
// is then scanned as its own plain IDENT. See lexer.go's numberIsGlued for
// the implementation and full rationale.
func TestDurationCompoundSplit(t *testing.T) {
	toks := lexAll(t, "1h30m")
	assertKinds(t, toks, []token.Kind{token.DURATION, token.INT, token.IDENT, token.EOF})

	want := []string{"1h", "30", "m"}
	for i, lit := range want {
		if toks[i].Lit != lit {
			t.Fatalf("tok[%d].Lit = %q, want %q", i, toks[i].Lit, lit)
		}
	}
}

func TestStrings(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		wantLit string
	}{
		{"plain string", `"hello"`, "hello"},
		{"empty string", `""`, ""},
		{"escaped quote", `"say \"hi\""`, `say "hi"`},
		{"escaped backslash", `"a\\b"`, `a\b`},
		{"escaped newline", `"a\nb"`, "a\nb"},
		{"escaped tab", `"a\tb"`, "a\tb"},
		{"unrecognized escape kept literally", `"a\qb"`, `a\qb`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			assertKinds(t, toks, []token.Kind{token.STRING, token.EOF})

			if toks[0].Lit != tt.wantLit {
				t.Fatalf("Lit = %q, want %q", toks[0].Lit, tt.wantLit)
			}
		})
	}
}

func TestUnterminatedStringRecovers(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantLit   string
		afterKind token.Kind
		afterLit  string
	}{
		{"unterminated at EOF", `"abc`, "abc", token.EOF, ""},
		{"unterminated empty at EOF", `"`, "", token.EOF, ""},
		{"trailing backslash at EOF", `"abc\`, "abc", token.EOF, ""},
		{"unterminated before raw newline recovers to next token", "\"abc\ndef", "abc", token.IDENT, "def"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New("test.zen", []byte(tt.src))

			tok := l.Next()
			if tok.Kind != token.STRING {
				t.Fatalf("Kind = %s, want STRING", tok.Kind)
			}

			if tok.Lit != tt.wantLit {
				t.Fatalf("Lit = %q, want %q", tok.Lit, tt.wantLit)
			}

			if !l.Errors().HasErrors() {
				t.Fatalf("expected a diagnostic for the unterminated string, got none")
			}

			for _, d := range l.Errors() {
				if d.Phase != "lex" {
					t.Fatalf("diagnostic Phase = %q, want %q", d.Phase, "lex")
				}
			}

			next := l.Next()
			if next.Kind != tt.afterKind {
				t.Fatalf("recovery Kind = %s, want %s", next.Kind, tt.afterKind)
			}

			if next.Lit != tt.afterLit {
				t.Fatalf("recovery Lit = %q, want %q", next.Lit, tt.afterLit)
			}
		})
	}
}

func TestComments(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []token.Kind
	}{
		{"comment then newline then ident", "// hello\nfoo", []token.Kind{token.IDENT, token.EOF}},
		{"comment with no trailing newline at EOF", "// hello", []token.Kind{token.EOF}},
		{"empty comment with no trailing newline at EOF", "//", []token.Kind{token.EOF}},
		{"ident then comment then ident", "foo // trailing comment\nbar", []token.Kind{token.IDENT, token.IDENT, token.EOF}},
		{"comment-only line between tokens", "foo\n// comment\nbar", []token.Kind{token.IDENT, token.IDENT, token.EOF}},
		{"bare slash is illegal, not a comment", "/", []token.Kind{token.ILLEGAL, token.EOF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			assertKinds(t, toks, tt.want)
		})
	}
}

func TestIllegalCharsRecover(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []token.Kind
		lits []string
	}{
		{"hash", "#", []token.Kind{token.ILLEGAL, token.EOF}, []string{"#"}},
		{"dollar", "$", []token.Kind{token.ILLEGAL, token.EOF}, []string{"$"}},
		{"percent", "%", []token.Kind{token.ILLEGAL, token.EOF}, []string{"%"}},
		{
			"illegal then valid token",
			"# foo",
			[]token.Kind{token.ILLEGAL, token.IDENT, token.EOF},
			[]string{"#", "foo"},
		},
		{
			"multiple illegal chars in a row all recover -- lexer never stops",
			"#$%",
			[]token.Kind{token.ILLEGAL, token.ILLEGAL, token.ILLEGAL, token.EOF},
			[]string{"#", "$", "%"},
		},
		{
			"illegal chars interspersed with valid tokens",
			"foo # bar $ baz",
			[]token.Kind{token.IDENT, token.ILLEGAL, token.IDENT, token.ILLEGAL, token.IDENT, token.EOF},
			[]string{"foo", "#", "bar", "$", "baz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := New("test.zen", []byte(tt.src))

			var toks []token.Token

			for {
				tok := l.Next()
				toks = append(toks, tok)

				if tok.Kind == token.EOF {
					break
				}
			}

			assertKinds(t, toks, tt.want)

			for i, lit := range tt.lits {
				if toks[i].Lit != lit {
					t.Fatalf("tok[%d].Lit = %q, want %q", i, toks[i].Lit, lit)
				}
			}

			if !l.Errors().HasErrors() {
				t.Fatalf("expected diagnostics for illegal characters, got none")
			}

			for _, d := range l.Errors() {
				if d.Phase != "lex" {
					t.Fatalf("diagnostic Phase = %q, want %q", d.Phase, "lex")
				}
			}
		})
	}
}

func TestIllegalCharDiagnosticPosition(t *testing.T) {
	l := New("test.zen", []byte("ok #bad"))

	first := l.Next()
	if first.Kind != token.IDENT {
		t.Fatalf("first token = %s, want IDENT", first.Kind)
	}

	second := l.Next()
	if second.Kind != token.ILLEGAL {
		t.Fatalf("second token = %s, want ILLEGAL", second.Kind)
	}

	errs := l.Errors()
	if len(errs) != 1 {
		t.Fatalf("errors = %d, want 1", len(errs))
	}

	if errs[0].Pos.File != "test.zen" || errs[0].Pos.Line != 1 || errs[0].Pos.Col != 4 {
		t.Fatalf("diagnostic pos = %s:%d:%d, want test.zen:1:4", errs[0].Pos.File, errs[0].Pos.Line, errs[0].Pos.Col)
	}
}

// TestPositionTracking asserts line/col across a multi-line source,
// including a token on line 3+.
func TestPositionTracking(t *testing.T) {
	src := "entity User {\n  id: uuid\n  name: string\n}\n"
	l := New("test.zen", []byte(src))

	var toks []token.Token

	for {
		tok := l.Next()
		toks = append(toks, tok)

		if tok.Kind == token.EOF {
			break
		}
	}

	want := []struct {
		kind token.Kind
		lit  string
		line int
		col  int
	}{
		{token.ENTITY, "entity", 1, 1},
		{token.IDENT, "User", 1, 8},
		{token.LBRACE, "{", 1, 13},
		{token.IDENT, "id", 2, 3},
		{token.COLON, ":", 2, 5},
		{token.IDENT, "uuid", 2, 7},
		{token.IDENT, "name", 3, 3},
		{token.COLON, ":", 3, 7},
		{token.IDENT, "string", 3, 9},
		{token.RBRACE, "}", 4, 1},
	}

	if len(toks) < len(want) {
		t.Fatalf("token count = %d, want at least %d; got %v", len(toks), len(want), kinds(toks))
	}

	for i, w := range want {
		got := toks[i]
		if got.Kind != w.kind || got.Lit != w.lit {
			t.Fatalf("tok[%d] = %s %q, want %s %q", i, got.Kind, got.Lit, w.kind, w.lit)
		}

		if got.Pos.Line != w.line || got.Pos.Col != w.col {
			t.Fatalf("tok[%d] (%q) pos = %d:%d, want %d:%d", i, got.Lit, got.Pos.Line, got.Pos.Col, w.line, w.col)
		}
	}
}

// TestPositionTracksUTF16ColumnNotRuneColumn guards against a regression
// where Col counted runes: a diag.Position ends up reported verbatim as an
// LSP Position, which is UTF-16-code-unit based, so any astral-plane rune
// (outside the BMP, e.g. an emoji -- two UTF-16 units but one rune) earlier
// on a line must push every later column on that line by 2, not 1.
func TestPositionTracksUTF16ColumnNotRuneColumn(t *testing.T) {
	// Columns (1-based): `"` at 1, the emoji rune at 2 (one rune, but 2
	// UTF-16 units so the next column is 4, not 3), closing `"` at 4 (UTF-16
	// col), space at 5, `x` at 6.
	src := `"😀" x`
	toks := lexAll(t, src)

	want := []struct {
		kind token.Kind
		col  int
	}{
		{token.STRING, 1},
		{token.IDENT, 6},
	}

	for i, w := range want {
		if toks[i].Kind != w.kind {
			t.Fatalf("tok[%d].Kind = %s, want %s", i, toks[i].Kind, w.kind)
		}

		if toks[i].Pos.Col != w.col {
			t.Fatalf("tok[%d] (%s) Col = %d, want %d (UTF-16 code units, not runes)",
				i, toks[i].Kind, toks[i].Pos.Col, w.col)
		}
	}
}

func TestPunctuationRoundTrip(t *testing.T) {
	tests := []struct {
		src  string
		kind token.Kind
	}{
		{"{", token.LBRACE},
		{"}", token.RBRACE},
		{"(", token.LPAREN},
		{")", token.RPAREN},
		{":", token.COLON},
		{",", token.COMMA},
		{"->", token.ARROW},
		{"@", token.AT},
		{"-", token.MINUS},
		{"?", token.QUESTION},
	}

	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			assertKinds(t, toks, []token.Kind{tt.kind, token.EOF})

			if toks[0].Lit != tt.src {
				t.Fatalf("Lit = %q, want %q", toks[0].Lit, tt.src)
			}
		})
	}
}

func TestMinusVsArrowDisambiguation(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []token.Kind
	}{
		{"bare minus before comma", "-,", []token.Kind{token.MINUS, token.COMMA, token.EOF}},
		{"arrow", "->", []token.Kind{token.ARROW, token.EOF}},
		{"minus then digit (unary minus is a parser concern)", "-5", []token.Kind{token.MINUS, token.INT, token.EOF}},
		{"arrow followed by unrecognized char", "->>", []token.Kind{token.ARROW, token.ILLEGAL, token.EOF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := lexAll(t, tt.src)
			assertKinds(t, toks, tt.want)
		})
	}
}

// TestNoPanicOnTruncatedInput proves the single most important property of
// this lexer: it never panics and never stops (hangs) regardless of how
// malformed or truncated the input is, including mid-token EOF and invalid
// UTF-8 byte sequences.
func TestNoPanicOnTruncatedInput(t *testing.T) {
	tests := []struct {
		name string
		src  []byte
	}{
		{"unterminated string at eof", []byte(`"`)},
		{"trailing backslash in string at eof", []byte(`"abc\`)},
		{"trailing dot after int", []byte("1.")},
		{"leading dot before int", []byte(".5")},
		{"duration unit at eof", []byte("1h")},
		{"minutes unit at eof", []byte("5m")},
		{"comment with no newline at eof", []byte("//")},
		{"lone illegal char at eof", []byte("#")},
		{"trailing minus at eof", []byte("-")},
		{"truncated 2-byte utf8 lead byte", []byte{0xC2}},
		{"truncated 3-byte utf8 sequence", []byte{0xE2, 0x82}},
		{"lone continuation byte", []byte{0x80}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Next() panicked on input %q: %v", tt.src, r)
				}
			}()

			l := New("test.zen", tt.src)

			for i := 0; i < 100; i++ {
				tok := l.Next()
				if tok.Kind == token.EOF {
					return
				}
			}

			t.Fatalf("lexer did not reach EOF within 100 calls on input %q (possible infinite loop)", tt.src)
		})
	}
}

func TestNextAfterEOFStaysEOF(t *testing.T) {
	l := New("test.zen", []byte("x"))

	first := l.Next()
	if first.Kind != token.IDENT {
		t.Fatalf("first = %s, want IDENT", first.Kind)
	}

	for i := 0; i < 3; i++ {
		tok := l.Next()
		if tok.Kind != token.EOF {
			t.Fatalf("call %d after input exhausted = %s, want EOF", i, tok.Kind)
		}
	}
}

func TestNewAndErrorsEmptyByDefault(t *testing.T) {
	l := New("test.zen", []byte("foo"))
	if len(l.Errors()) != 0 {
		t.Fatalf("Errors() should be empty before any illegal input, got %v", l.Errors())
	}
}

// TestAdvanceAtEOFIsNoOp covers Lexer.advance's at-EOF early return: pushing
// past exhaustion must not panic, move the cursor, or disturb the sticky EOF.
func TestAdvanceAtEOFIsNoOp(t *testing.T) {
	l := New("test.zen", []byte("x"))

	if tok := l.Next(); tok.Kind != token.IDENT {
		t.Fatalf("first = %s, want IDENT", tok.Kind)
	}

	if tok := l.Next(); tok.Kind != token.EOF {
		t.Fatalf("second = %s, want EOF", tok.Kind)
	}

	l.advance() // must be a silent no-op, not a panic or cursor move

	if tok := l.Next(); tok.Kind != token.EOF {
		t.Fatalf("after advance at EOF = %s, want EOF", tok.Kind)
	}
}

// TestNumberIsGlued pins the glued-number guard scanNumber relies on to split
// compound durations ("1h30m" -> DURATION INT IDENT): a digit run counts as
// glued only when the byte before it is an identifier-start letter. The
// out-of-range/negative cases cover the defensive guard so a corrupt start
// reports "not glued" instead of panicking on a bad slice.
func TestNumberIsGlued(t *testing.T) {
	tests := []struct {
		name  string
		src   string
		start int
		want  bool
	}{
		{"start of input is never glued", "5m", 0, false},
		{"letter predecessor is glued", "1h30m", 2, true},
		{"digit predecessor is not glued", "123", 1, false},
		{"punctuation predecessor is not glued", ": 5m", 2, false},
		{"whitespace predecessor is not glued", "5m 30", 3, false},
		{"invalid utf8 tail predecessor is not glued", "a\xff30", 2, false},
		{"negative start is not glued", "abc", -1, false},
		{"start past end is not glued", "ab", 5, false},
		{"start at end after letter is glued", "ab", 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := numberIsGlued([]byte(tt.src), tt.start); got != tt.want {
				t.Fatalf("numberIsGlued(%q, %d) = %v, want %v", tt.src, tt.start, got, tt.want)
			}
		})
	}
}
