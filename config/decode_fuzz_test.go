package config

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzDecodeFile proves the YAML/JSON file decode path never panics on
// arbitrary input: decodeFile must return a clean error (or a clean decode)
// for any byte sequence. It runs as an ordinary seed-corpus regression
// test under `go test ./config/`; run with `-fuzz=FuzzDecodeFile
// -fuzztime=...` for continuous fuzzing.
func FuzzDecodeFile(f *testing.F) {
	for _, seed := range []string{
		"",
		"{}",
		"[]",
		"null",
		"adapter: sqlite",
		"db:\n  adapter: sqlite\n  options:\n    dsn: \":memory:\"\n",
		"db: [",
		"db: {adapter: sqlite, options: {dsn: 1}}",
		"\x00\x01\x02",
		"{\"db\": ",
		"db:\n\tadapter: tab-indent",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		t.Parallel()

		path := filepath.Join(t.TempDir(), "zever.yaml")

		if err := os.WriteFile(path, []byte(input), 0o600); err != nil {
			t.Fatalf("write seed: %v", err)
		}

		// The only acceptable outcomes are a clean decode or a clean error;
		// a panic fails the fuzz test.
		_, _ = decodeFile(path)
	})
}
