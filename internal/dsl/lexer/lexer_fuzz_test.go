package lexer

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/internal/dsl/token"
)

// repoFixture reads a file at rel, relative to the module root, using the
// caller's own source location to find the root regardless of the working
// directory `go test` was invoked from. It reports whether the fixture
// exists: not every fixture path from the source repo exists in this module
// (e.g. no examples/ tree yet), and a missing fixture must skip rather than
// fail the seed setup.
func repoFixture(f *testing.F, rel string) (string, bool) {
	f.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		f.Fatalf("runtime.Caller failed while locating fixture %q", rel)
	}

	// internal/dsl/lexer -> repo root is three levels up.
	root := filepath.Join(filepath.Dir(file), "..", "..", "..")

	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		f.Logf("skipping missing fixture %q: %v", rel, err)
		return "", false
	}

	return string(data), true
}

// FuzzLex proves the lexer's documented "never panics, never stops"
// invariant (see the package doc comment) mechanically rather than by
// hand-written test case alone: Next() must terminate at EOF for any input,
// including invalid UTF-8, without ever recovering from a panic.
func FuzzLex(f *testing.F) {
	// Seeds extracted from lexer_test.go's own hand-written cases.
	seeds := []string{
		"",
		`entity User { id: uuid }`,
		`rpc Get(id: uuid) -> User`,
		`@auth`,
		`interval: 5m`,
		`backoff(base: 30s)`,
		"entity", "service", "job", "schedule", "rpc", "index", "enum",
		"has_many", "has_one", "belongs_to", "many_to_many", "message",
		"true", "false", "uuid", "string", "int64", "timestamp",
		"http", "auth", "permission", "queue", "retry", "cron", "dispatch",
		"join_table", "GET", "POST", "PUT", "DELETE", "PATCH",
		"42", "0", "3.14", "1.", ".5", "1.foo", "1..2",
		"500ns", "10us", "10µs", "100ms", "30s", "5m", "1h", "5ms", "5us",
		"1h30m",
		`"hello"`, `""`, `"say \"hi\""`, `"a\\b"`, `"a\nb"`, `"a\tb"`, `"a\qb"`,
		`"abc`, `"`, `"abc\`, "\"abc\ndef",
		"// hello\nfoo", "// hello", "//", "foo // trailing comment\nbar",
		"foo\n// comment\nbar", "/",
		"// doc line 1\n// doc line 2\nentity User {\n  id: uuid\n}",
		"// doc\n\nentity User {}", "entity User {} // trailing, no newline",
		"#", "$", "%", "# foo", "#$%", "foo # bar $ baz",
		"ok #bad",
		"entity User {\n  id: uuid\n  name: string\n}\n",
		"{", "}", "(", ")", ":", ",", "->", "-", "?",
		"-,", "->", "-5", "->>",
		"1h", "5m",
		// Astral-plane runes (outside BMP, 2 UTF-16 units, 4 UTF-8 bytes) — verifies col counts 1 per rune.
		"💖", "😀", "🧪", "entity User { name: \"💖\" }", "// 💖 astral comment\nentity User { id: uuid }",
		"a 💖 b", "\"💖\"", "id: \"😀\" // 😀 trailing",
		// Deep nesting — verifies lexer terminates without blowup.
		"{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{{}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}}",
		"(((((((((((((((((((((((((((((())))))))))))))))))))))))))))))",
		// 10k-depth nesting — R28/R29 corpus gap: lexer must still reach EOF without panic or hang.
		strings.Repeat("{", 10000) + strings.Repeat("}", 10000),
		strings.Repeat("(", 10000) + strings.Repeat(")", 10000),
		strings.Repeat("[", 10000) + strings.Repeat("]", 10000),
		// Astral emoji alone and repeated — ensures UTF-16 col counting stays 1 per rune even at scale.
		"😀",
		strings.Repeat("😀", 100),
		"entity User { name: string @default(\"😀\") } // 😀 astral emoji corpus",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	// Deliberately-malformed / adversarial inputs to bias coverage toward
	// the interesting parts of the state machine faster.
	malformed := []string{
		`"unterminated string with no end`,
		"/* not a real comment style, lone slash-star */",
		"@@@???",
		"entity {{{{{{{{{{{{{{{{{{{{",
		"}}}}}}}}}}}}}}}}}}}}",
		"entity User { id: uuid",    // missing closing brace
		"rpc Get(id: uuid",          // missing closing paren
		"errors: { not_found,",      // unbalanced braces
		"1" + string(rune(0)) + "2", // embedded NUL
		"\xff\xfe\xfd",              // invalid UTF-8 bytes
		string([]byte{0xC2}),        // truncated 2-byte UTF-8 lead byte
		string([]byte{0xE2, 0x82}),  // truncated 3-byte UTF-8 sequence
		string([]byte{0x80}),        // lone continuation byte
		"a" + string([]byte{0xC0, 0x80}) + "b",
		"----------------------------",
		"?????????????????????????????",
		// 10k-depth unbalanced — lexer must not hang, must emit ILLEGAL/EOF eventually.
		strings.Repeat("{", 10000),
		strings.Repeat("}", 10000),
		strings.Repeat("😀", 500) + strings.Repeat("{", 5000),
	}
	for _, s := range malformed {
		f.Add(s)
	}

	// Real .zen fixtures used elsewhere in the repo (skipped when absent).
	for _, rel := range []string{
		"internal/dsl/testdata/todo.zen",
		"internal/dsl/testdata/demoapp.zen",
		"internal/dsl/compile/testdata/app.zen",
	} {
		if src, ok := repoFixture(f, rel); ok {
			f.Add(src)
		}
	}

	f.Fuzz(func(t *testing.T, src string) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Lex panicked on input %q: %v", src, r)
			}
		}()

		l := New("fuzz.zen", []byte(src))

		// A well-behaved lexer reaches EOF in at most len(src)+1 calls,
		// since every call to Next consumes at least one byte (or reports
		// EOF outright). Cap iterations generously above that so a
		// (never-panicking, but hypothetically non-terminating) bug shows
		// up as a fast, actionable test failure instead of hanging the
		// whole fuzz run or CI.
		limit := len(src)*2 + 64
		for i := 0; i <= limit; i++ {
			tok := l.Next()
			if tok.Kind == token.EOF {
				return
			}

			if i == limit {
				t.Fatalf("Lex did not reach EOF within %d calls on input %q (possible infinite loop)", limit, src)
			}
		}
	})
}
