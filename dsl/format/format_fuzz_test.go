package format

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// FuzzFormatIdempotent checks the two contracts the formatter documents:
// formatting is idempotent (Format(Format(x)) == Format(x)), and it only
// moves whitespace (the token stream is unchanged). Lex-dirty input is
// refused with ErrDirty and is not a failure.
func FuzzFormatIdempotent(f *testing.F) {
	seeds := []string{
		"",
		"entity    User   {\nid:   uuid   @primary\n}\n",
		"entity User {\nid:uuid @ primary\n        email : string\n  }\n",
		"service S {\n  rpc Get(id: uuid) -> User @http(GET, \"/u/{id}\")\n}\n",
		"job J { queue: \"q\" retry: backoff(base: 30s) }\n",
		"// comment\nentity A { x: string? @default(\"a\") }\n",
		"entity E {\r\n\tid: uuid\r\n}\r\n",
		"entity X { f: \"unclosed",
		"\xff\xfe entity",
	}

	if data, err := os.ReadFile(filepath.Join("..", "compile", "testdata", "app.zen")); err == nil {
		seeds = append(seeds, string(data))
	}

	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, src []byte) {
		once, err := Format(src)
		if err != nil {
			if !errors.Is(err, ErrDirty) {
				t.Fatalf("Format: unexpected error %v", err)
			}

			return
		}

		twice, err := Format(once)
		if err != nil {
			t.Fatalf("Format(Format(x)) failed on formatted output: %v\ninput: %q\nonce: %q", err, src, once)
		}

		if string(once) != string(twice) {
			t.Fatalf("not idempotent\ninput: %q\nonce:  %q\ntwice: %q", src, once, twice)
		}

		before, ok1 := LexAll("in", src)
		after, ok2 := LexAll("out", once)

		if !ok1 || !ok2 {
			t.Fatalf("lex errors around a successful format (in clean=%v, out clean=%v)\ninput: %q\nonce: %q", ok1, ok2, src, once)
		}

		if len(before) != len(after) {
			t.Fatalf("token count changed %d -> %d\ninput: %q\nonce: %q", len(before), len(after), src, once)
		}

		for i := range before {
			if before[i].Kind != after[i].Kind || before[i].Lit != after[i].Lit {
				t.Fatalf("token %d changed: %v %q -> %v %q\ninput: %q\nonce: %q",
					i, before[i].Kind, before[i].Lit, after[i].Kind, after[i].Lit, src, once)
			}
		}
	})
}
