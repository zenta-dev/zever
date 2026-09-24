package static

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/flag"
)

func openTestdata(t *testing.T, reload bool) flag.Flag {
	t.Helper()
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: "testdata/flags.json", Reload: reload}})
	if err != nil {
		t.Fatalf("New(testdata): %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func writeTempFlags(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "flags.json")
	//nolint:gosec // test-only path under TempDir.
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp flags: %v", err)
	}
	return p
}

func bumpModtime(t *testing.T, path string, add time.Duration) {
	t.Helper()
	ts := time.Now().Add(add)
	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

func TestLoadTypedReads(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	b, err := f.Bool(ctx, "debug", false)
	if err != nil || !b {
		t.Fatalf("Bool(debug) = %v, %v; want true, nil", b, err)
	}
	s, err := f.String(ctx, "env", "dev")
	if err != nil || s != "prod" {
		t.Fatalf("String(env) = %q, %v; want %q, nil", s, err, "prod")
	}
	n, err := f.Int(ctx, "max_retries", 0)
	if err != nil || n != 3 {
		t.Fatalf("Int(max_retries) = %d, %v; want 3, nil", n, err)
	}
}

func TestMissingReturnsFallbackNilErr(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	if b, err := f.Bool(ctx, "nope", true); err != nil || !b {
		t.Fatalf("Bool(missing) = %v, %v; want true, nil", b, err)
	}
	if s, err := f.String(ctx, "nope", "fb"); err != nil || s != "fb" {
		t.Fatalf("String(missing) = %q, %v; want %q, nil", s, err, "fb")
	}
	if n, err := f.Int(ctx, "nope", 7); err != nil || n != 7 {
		t.Fatalf("Int(missing) = %d, %v; want 7, nil", n, err)
	}
	var out map[string]any
	if err := f.JSON(ctx, "nope", &out, nil); err != nil {
		t.Fatalf("JSON(missing, nil fallback) = %v; want nil", err)
	}
}

func TestMismatchErrorHasKeyAndType(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	if _, err := f.Bool(ctx, "env", false); err == nil ||
		!strings.Contains(err.Error(), `"env"`) ||
		!strings.Contains(err.Error(), "bool") {
		t.Fatalf("Bool(env) err = %v; want key+bool", err)
	}
	if _, err := f.String(ctx, "config", "fb"); err == nil ||
		!strings.Contains(err.Error(), `"config"`) ||
		!strings.Contains(err.Error(), "string") {
		t.Fatalf("String(config) err = %v; want key+string", err)
	}
	if _, err := f.Int(ctx, "env", 0); err == nil ||
		!strings.Contains(err.Error(), `"env"`) ||
		!strings.Contains(err.Error(), "int") {
		t.Fatalf("Int(env) err = %v; want key+int", err)
	}
}

func TestStrictBool(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	if b, err := f.Bool(ctx, "enabled_str", false); err != nil || !b {
		t.Fatalf("Bool(enabled_str=TRUE) = %v, %v", b, err)
	}
	if b, err := f.Bool(ctx, "off_str", true); err != nil || b {
		t.Fatalf("Bool(off_str=false) = %v, %v", b, err)
	}
	for _, k := range []string{"one_str", "yes_str"} {
		if _, err := f.Bool(ctx, k, false); err == nil {
			t.Fatalf("Bool(%s) = nil error; want strict reject", k)
		}
	}
}

func TestIntFromStringAndFloatReject(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	if n, err := f.Int(ctx, "timeout_str", 0); err != nil || n != 30 {
		t.Fatalf("Int(timeout_str) = %d, %v; want 30", n, err)
	}
	if _, err := f.Int(ctx, "float_val", 0); err == nil {
		t.Fatal("Int(3.5) = nil error; want truncation reject")
	}
	if _, err := f.Int(ctx, "pi_str", 0); err == nil {
		t.Fatal(`Int("3.14") = nil error; want reject`)
	}
}

func TestIntOverflowAndPrecision(t *testing.T) {
	t.Parallel()
	p := writeTempFlags(t, `{"huge": 1e19, "prec": 9007199254740993}`)
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	ctx := t.Context()

	if _, err := f.Int(ctx, "huge", 0); err == nil {
		t.Fatal("Int(1e19) = nil error; want overflow")
	}
	if _, err := f.Int(ctx, "prec", 0); err == nil {
		t.Fatal("Int(2^53+1) = nil error; want precision reject")
	}
}

func TestNumberToStringFormatFloat(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	s, err := f.String(ctx, "big_num", "fb")
	if err != nil || s != "1000000" {
		t.Fatalf("String(big_num) = %q, %v; want %q", s, err, "1000000")
	}
	s, err = f.String(ctx, "count_num", "fb")
	if err != nil || s != "1234" {
		t.Fatalf("String(count_num) = %q, %v; want 1234", s, err)
	}
	s, err = f.String(ctx, "debug", "fb")
	if err != nil || s != "true" {
		t.Fatalf("String(debug bool) = %q, %v; want true", s, err)
	}
}

func TestJSONRawStringAndObject(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	var raw struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := f.JSON(ctx, "raw_json", &raw, nil); err != nil {
		t.Fatalf("JSON(raw_json): %v", err)
	}
	if raw.Host != "x" || raw.Port != 1 {
		t.Fatalf("JSON(raw_json) = %+v; want {x 1}", raw)
	}

	var cfg struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := f.JSON(ctx, "config", &cfg, nil); err != nil {
		t.Fatalf("JSON(config): %v", err)
	}
	if cfg.Host != "db.local" || cfg.Port != 5432 {
		t.Fatalf("JSON(config) = %+v", cfg)
	}
}

func TestJSONFallbackFill(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	var out struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	fb := map[string]any{"host": "fb.local", "port": float64(1)}
	if err := f.JSON(ctx, "nope", &out, fb); err != nil {
		t.Fatalf("JSON(missing, fallback): %v", err)
	}
	if out.Host != "fb.local" || out.Port != 1 {
		t.Fatalf("JSON fallback = %+v", out)
	}
}

func TestJSONNilOut(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	if err := f.JSON(t.Context(), "config", nil, nil); err == nil {
		t.Fatal("JSON(nil out) = nil; want error")
	}
}

func TestReloadPicksUpChange(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/flags.json")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	p := writeTempFlags(t, string(data))
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	ctx := t.Context()

	if s, _ := f.String(ctx, "env", ""); s != "prod" {
		t.Fatalf("before reload env = %q", s)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	m["env"] = "staging"
	changed, _ := json.Marshal(m)
	if err := os.WriteFile(p, changed, 0o600); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	bumpModtime(t, p, 2*time.Second)

	if s, err := f.String(ctx, "env", ""); err != nil || s != "staging" {
		t.Fatalf("after reload env = %q, %v; want staging", s, err)
	}
}

func TestReloadKeepsLastGoodOnBadJSON(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("testdata/flags.json")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	p := writeTempFlags(t, string(data))
	f, err := New(flag.Options{Static: flag.StaticOptions{Path: p, Reload: true}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	ctx := t.Context()

	if err := os.WriteFile(p, []byte("{bad json"), 0o600); err != nil {
		t.Fatalf("rewrite bad: %v", err)
	}
	bumpModtime(t, p, 2*time.Second)

	if s, err := f.String(ctx, "env", "fb"); err != nil || s != "prod" {
		t.Fatalf("bad reload env = %q, %v; want last-good prod", s, err)
	}
}

func TestEmptyPathDevDefault(t *testing.T) {
	t.Parallel()
	f, err := New(flag.Options{})
	if err != nil {
		t.Fatalf("New(empty) = %v; want nil", err)
	}
	t.Cleanup(func() { _ = f.Close() })
	ctx := t.Context()

	if b, err := f.Bool(ctx, "any", true); err != nil || !b {
		t.Fatalf("empty Bool = %v, %v", b, err)
	}
	if s, err := f.String(ctx, "any", "fb"); err != nil || s != "fb" {
		t.Fatalf("empty String = %q, %v", s, err)
	}
}

func TestBadPathErrors(t *testing.T) {
	t.Parallel()

	if _, err := New(flag.Options{Static: flag.StaticOptions{Path: "testdata/does-not-exist.json"}}); err == nil {
		t.Fatal("New(missing) = nil; want load error")
	} else {
		var ioerr *flag.InvalidOptionsError
		if errors.As(err, &ioerr) {
			t.Fatalf("New(missing) = %v; want load error, not InvalidOptions", err)
		}
	}

	dir := t.TempDir()
	txt := filepath.Join(dir, "flags.txt")
	if werr := os.WriteFile(txt, []byte(`{}`), 0o600); werr != nil {
		t.Fatalf("seed txt: %v", werr)
	}

	for _, tc := range []struct {
		name string
		path string
	}{
		{"ext", txt},
		{"traversal", filepath.Join("..", "flags.json")},
	} {
		_, nerr := New(flag.Options{Static: flag.StaticOptions{Path: tc.path}})
		var ioerr *flag.InvalidOptionsError
		if nerr == nil || !errors.As(nerr, &ioerr) {
			t.Fatalf("New(%s %q) = %v; want *InvalidOptionsError", tc.name, tc.path, nerr)
		}
	}

	// absolute paths are allowed (operator configuration); a missing
	// absolute file fails at load, not validation.
	if _, nerr := New(flag.Options{Static: flag.StaticOptions{Path: filepath.Join(string(filepath.Separator), "tmp", "flags.json")}}); nerr == nil {
		t.Fatal("New(abs missing) = nil; want load error")
	} else {
		var ioerr *flag.InvalidOptionsError
		if errors.As(nerr, &ioerr) {
			t.Fatalf("New(abs missing) = %v; want load error, not InvalidOptions", nerr)
		}
	}

	// directory carrying a .json suffix
	dj := t.TempDir() + ".json"
	if err := os.Mkdir(dj, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dj) })
	if _, err := New(flag.Options{Static: flag.StaticOptions{Path: dj}}); err == nil {
		t.Fatalf("New(dir %q) = nil; want error", dj)
	} else {
		var ioerr *flag.InvalidOptionsError
		if !errors.As(err, &ioerr) {
			t.Fatalf("New(dir) = %v; want *InvalidOptionsError", err)
		}
	}
}

func TestInvalidKey(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx := t.Context()

	if _, err := f.Bool(ctx, "", false); err == nil {
		t.Fatal("empty key = nil; want error")
	}
	if _, err := f.String(ctx, "bad\nkey", "fb"); err == nil {
		t.Fatal("control-char key = nil; want error")
	}
	var serr *flag.InvalidKeyError
	if _, err := f.Int(ctx, "", 0); !errors.As(err, &serr) {
		t.Fatalf("Int(empty key) err type = %T; want *InvalidKeyError", err)
	}
}

func TestCtxCanceled(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := f.Bool(ctx, "debug", false); err == nil {
		t.Fatal("canceled Bool = nil; want ctx err")
	}
	if _, err := f.String(ctx, "env", ""); err == nil {
		t.Fatal("canceled String = nil; want ctx err")
	}
	if _, err := f.Int(ctx, "max_retries", 0); err == nil {
		t.Fatal("canceled Int = nil; want ctx err")
	}
	var out map[string]any
	if err := f.JSON(ctx, "config", &out, nil); err == nil {
		t.Fatal("canceled JSON = nil; want ctx err")
	}
}

func TestCloseNilOp(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, false)
	if err := f.Close(); err != nil {
		t.Fatalf("Close = %v; want nil", err)
	}
	if b, err := f.Bool(t.Context(), "debug", false); err != nil || !b {
		t.Fatalf("after Close Bool = %v, %v", b, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("second Close = %v", err)
	}
}

func TestConcurrentReads(t *testing.T) {
	t.Parallel()
	f := openTestdata(t, true)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := t.Context()
			_, _ = f.Bool(ctx, "debug", false)
			_, _ = f.String(ctx, "env", "")
			_, _ = f.Int(ctx, "max_retries", 0)
			var out map[string]any
			_ = f.JSON(ctx, "config", &out, nil)
		}()
	}
	wg.Wait()
}
