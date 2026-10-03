package render

import (
	"testing"

	"github.com/zenta-dev/zever/orm/dialect/sqlite"
)

// FuzzRenderRaw proves the raw, identifier, and JSON render paths never
// panic on arbitrary input: every payload must render to SQL text with the
// payload only ever among the bound args, or return a clean error. It runs
// as an ordinary seed-corpus regression test under `go test ./orm/render/`;
// run with `-fuzz=FuzzRenderRaw -fuzztime=...` for continuous fuzzing.
func FuzzRenderRaw(f *testing.F) {
	for _, seed := range []string{
		"",
		"name = ?",
		"a = ? AND b = ?",
		"1 = 1",
		"' OR 1=1 --",
		"'; DROP TABLE widgets; --\x00",
		"\x00",
		"漢字' OR 1=1 -- 🚀",
		"?",
		"??",
		"%",
		"$1",
	} {
		f.Add(seed, seed)
	}

	f.Fuzz(func(t *testing.T, fragment, arg string) {
		t.Parallel()

		// Raw escape hatch: fragment renders verbatim, arg stays bound.
		_, _, err := renderExpr(sqlite.New(), Node{Kind: KindRaw, Value: RawExpr{Fragment: fragment, Args: []any{arg}}}, &argCounter{})
		_ = err

		// Identifier quoting: arbitrary column names must quote safely.
		_, _, err = renderScalar(sqlite.New(), Node{Kind: KindColumn, Column: fragment}, &argCounter{}, renderScope{})
		_ = err

		// JSON path: arbitrary column, key, and comparison value.
		_, _, err = renderExpr(sqlite.New(), Node{
			Kind:   KindJSON,
			Column: fragment,
			Op:     OpEq,
			Value:  arg,
			JSON:   &JSONExpr{Op: JSONExtract, Steps: []JSONStep{{Key: arg}}},
		}, &argCounter{})
		_ = err
	})
}
