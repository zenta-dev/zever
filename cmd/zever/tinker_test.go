package main

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/traefik/yaegi/stdlib"
)

var errTestSentinel = errors.New("test sentinel read error")

// TestNormalizeTinkerSource pins the yaegi workaround: an immediately-invoked
// function literal is parenthesized (yaegi 0.16.1 compiles the bare form as a
// file-level declaration and loses the call's value), while declarations and
// unrelated lines pass through untouched.
func TestNormalizeTinkerSource(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{`func() string { return "z" }()`, `(func() string { return "z" }())`},
		{`  func() int { return 1 }()  `, `(func() int { return 1 }())`},
		{`func() {}`, `(func() {})`},
		{`func f() {}`, `func f() {}`},
		{`funcs`, `funcs`},
		{`cache.Get("k")`, `cache.Get("k")`},
		{`1 + 2`, `1 + 2`},
		{`this is not go`, `this is not go`},
	}

	for _, tc := range cases {
		if got := normalizeTinkerSource(tc.in); got != tc.want {
			t.Errorf("normalizeTinkerSource(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestTinkerEvalReturnsValue proves interpreted expressions evaluate and
// their values survive unwrapping.
func TestTinkerEvalReturnsValue(t *testing.T) {
	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	v, err := tinkerEval(i, `func() string { s := "zen"; return strings.ToUpper(s) }()`)
	if err != nil {
		t.Fatalf("tinkerEval: %v", err)
	}

	if got := formatTinkerValue(v); got != "ZEN" {
		t.Fatalf("func literal value = %q, want %q", got, "ZEN")
	}
}

// TestTinkerEvalReportsError proves a syntax error surfaces instead of
// panicking or hanging.
func TestTinkerEvalReportsError(t *testing.T) {
	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	if _, err := tinkerEval(i, `this is not go`); err == nil {
		t.Fatal("expected an error for invalid Go, got nil")
	}
}

// TestTinkerEvalTimeout proves evaluation is bounded: an infinite loop is
// interrupted instead of hanging the REPL forever.
func TestTinkerEvalTimeout(t *testing.T) {
	orig := tinkerEvalTimeout
	tinkerEvalTimeout = 200 * time.Millisecond
	t.Cleanup(func() { tinkerEvalTimeout = orig })

	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	start := time.Now()
	_, err = tinkerEval(i, `func() { for {} }()`)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error for an infinite loop, got nil")
	}

	if elapsed > 30*time.Second {
		t.Fatalf("eval took %v, timeout did not bound it", elapsed)
	}
}

// TestTinkerBlocksOsExec is the negative security test: importing os/exec
// in the REPL must fail. os/exec would let interpreted code spawn arbitrary
// host processes outside the shim protocol.
func TestTinkerBlocksOsExec(t *testing.T) {
	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	if _, err := i.Eval(`import "os/exec"`); err == nil {
		t.Fatal(`expected 'import "os/exec"' to fail, got nil`)
	}
}

// TestTinkerBlocksSyscallAndUnsafe proves the rest of the denylist: raw
// syscalls and unsafe pointer games have no place in a REPL.
func TestTinkerBlocksSyscallAndUnsafe(t *testing.T) {
	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	for _, src := range []string{`import "syscall"`, `import "unsafe"`} {
		if _, err := i.Eval(src); err == nil {
			t.Fatalf("expected %q to fail, got nil", src)
		}
	}
}

// TestTinkerAllowedSymbolsExcludesDangerousPackages pins the allowlist at
// the symbol-table level, independent of interpreter behavior.
func TestTinkerAllowedSymbolsExcludesDangerousPackages(t *testing.T) {
	t.Parallel()

	syms := tinkerAllowedSymbols()

	for key := range syms {
		pkg := symKeyImportPath(key)
		switch pkg {
		case "os/exec", "syscall", "unsafe", "plugin":
			t.Fatalf("allowlist exposes dangerous package %q (key %q)", pkg, key)
		}
	}

	if len(syms) == 0 {
		t.Fatal("allowlist must not be empty")
	}
}

// TestTinkerREPLLoop drives the REPL's line handling without a terminal or a
// shim: container calls fail fast against the closed client and the loop
// must survive them.
func TestTinkerREPLLoop(t *testing.T) {
	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	var out strings.Builder

	input := strings.NewReader(strings.Join([]string{
		``,
		`1 + 2`,
		`this is not go`,
		`cache.Get("repl")`,
		`:help`,
		`:exit`,
		`cache.Get("never reached")`,
	}, "\n"))

	if err := tinkerREPL(i, input, &out); err != nil {
		t.Fatalf("tinkerREPL: %v", err)
	}

	text := out.String()

	for _, want := range []string{"zever> ", "3", "error:"} {
		if !strings.Contains(text, want) {
			t.Errorf("REPL output missing %q:\n%s", want, text)
		}
	}

	if strings.Contains(text, "never reached") {
		t.Errorf(":exit did not stop the loop, output:\n%s", text)
	}
}

// TestRunTinkerWithoutShim: the failure a developer is most likely to hit
// first must name the fix.
func TestRunTinkerWithoutShim(t *testing.T) {
	t.Chdir(t.TempDir())

	err := runTinker(nil)
	if err == nil {
		t.Fatal("expected an error when no shim exists")
	}

	if !strings.Contains(err.Error(), "zever generate tinker") {
		t.Errorf("error should point at the scaffolder, got: %v", err)
	}
}

// TestTinkerDevWarningIsExplicit pins the dev-only banner: operators must be
// told this REPL is not a production tool.
func TestTinkerDevWarningIsExplicit(t *testing.T) {
	t.Parallel()

	if !strings.Contains(tinkerDevWarning, "development") {
		t.Fatalf("dev warning must say development-only, got: %q", tinkerDevWarning)
	}
}

// fakeTinkerShim is a self-contained protocol peer: it answers every verb
// with canned payloads, emits one non-protocol log line (passthrough), and
// can be told to misbehave per test via the request contents.
const fakeTinkerShim = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	fmt.Println("shim booting, not protocol")

	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for sc.Scan() {
		var req struct {
			Verb string          ` + "`json:\"verb\"`" + `
			Args json.RawMessage ` + "`json:\"args,omitempty\"`" + `
		}

		if err := json.Unmarshal([]byte(sc.Text()), &req); err != nil {
			continue
		}

		var result any
		var rerr string

		switch req.Verb {
		case "ping":
		case "db.query":
			var a struct {
				SQL string ` + "`json:\"sql\"`" + `
			}

			_ = json.Unmarshal(req.Args, &a)

			switch {
			case strings.Contains(a.SQL, "badframe"):
				fmt.Println("@@zever-tinker@@not-json{{{")
				continue
			case strings.Contains(a.SQL, "boom"):
				rerr = "boom: no such table"
			default:
				result = map[string]any{"columns": []string{"id"}, "rows": [][]any{{float64(1)}}}
			}
		case "db.exec":
		case "cache.get":
			result = map[string]any{"value": "hello"}
		case "cache.set", "cache.delete":
		case "cache.exists":
			result = map[string]any{"value": true}
		case "queue.push":
		case "queue.length":
			result = map[string]any{"value": float64(3)}
		case "job.dispatch":
		default:
			rerr = "unknown verb " + req.Verb
		}

		out, _ := json.Marshal(map[string]any{"result": result, "error": rerr})
		fmt.Println("@@zever-tinker@@" + string(out))
	}
}
`

// startFakeTinkerShim scaffolds a temp module running fakeTinkerShim and
// returns a connected client. It needs the go toolchain, so callers skip in
// short mode.
func startFakeTinkerShim(t *testing.T) (*tinkerClient, *strings.Builder) {
	t.Helper()

	if testing.Short() {
		t.Skip("spawns a `go run` subprocess")
	}

	dir := t.TempDir()
	writeZeverFixture(t, dir, "go.mod", "module fakeshim\n\ngo 1.24\n")
	writeZeverFixture(t, dir, "main.go", fakeTinkerShim)
	t.Chdir(dir)

	var passthrough strings.Builder

	client, err := startTinkerShim(".", &passthrough)
	if err != nil {
		t.Fatalf("startTinkerShim: %v", err)
	}

	t.Cleanup(func() { _ = client.Close() })

	if err := client.ping(5 * time.Minute); err != nil {
		t.Fatalf("shim never became ready: %v", err)
	}

	return client, &passthrough
}

// TestTinkerShimRoundTrip drives every REPL-facing method through a real
// subprocess round trip, plus the error and bad-frame paths.
func TestTinkerShimRoundTrip(t *testing.T) {
	client, passthrough := startFakeTinkerShim(t)

	if !strings.Contains(passthrough.String(), "not protocol") {
		t.Fatalf("expected passthrough of non-protocol output, got %q", passthrough.String())
	}

	i, err := newTinkerInterp(client)
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	eval := func(src string) any {
		t.Helper()

		v, evalErr := tinkerEval(i, src)
		if evalErr != nil {
			t.Fatalf("eval %q: %v", src, evalErr)
		}

		v = unwrapTinkerValue(v)
		if !v.IsValid() || !v.CanInterface() {
			return nil
		}

		return v.Interface()
	}

	if got := eval(`cache.Set("k", "v", 60)`); got != nil {
		t.Fatalf("cache.Set = %#v, want nil", got)
	}

	if got := eval(`func() string { v, _ := cache.Get("k"); return v }()`); got != "hello" {
		t.Fatalf("cache.Get = %#v, want %q", got, "hello")
	}

	if got := eval(`func() bool { ok, _ := cache.Exists("k"); return ok }()`); got != true {
		t.Fatalf("cache.Exists = %#v, want true", got)
	}

	if got := eval(`cache.Delete("k")`); got != nil {
		t.Fatalf("cache.Delete = %#v, want nil", got)
	}

	if got := eval(`db.Exec("create table t (id integer)")`); got != nil {
		t.Fatalf("db.Exec = %#v, want nil", got)
	}

	rowsVal := eval(`func() []map[string]any { r, _ := db.Query("select id from t"); return r }()`)
	rows, ok := rowsVal.([]map[string]any)
	if !ok || len(rows) != 1 || rows[0]["id"] != float64(1) {
		t.Fatalf("db.Query = %#v, want one row with id 1", rowsVal)
	}

	if got := eval(`queue.Push("t", "p")`); got != nil {
		t.Fatalf("queue.Push = %#v, want nil", got)
	}

	if got := eval(`func() int64 { n, _ := queue.Length("t"); return n }()`); got != int64(3) {
		t.Fatalf("queue.Length = %#v, want 3", got)
	}

	if got := eval(`job.Dispatch("mail", map[string]any{"to": "a@b.c"})`); got != nil {
		t.Fatalf("job.Dispatch = %#v, want nil", got)
	}

	errText := eval(`func() string { _, err := db.Query("select boom"); if err == nil { return "" }; return err.Error() }()`)
	if s, _ := errText.(string); !strings.Contains(s, "boom") {
		t.Fatalf("expected shim error to surface, got %#v", errText)
	}

	frameErr := eval(`func() string { _, err := db.Query("badframe"); if err == nil { return "" }; return err.Error() }()`)
	if s, _ := frameErr.(string); !strings.Contains(s, "decode shim response") {
		t.Fatalf("expected frame decode error, got %#v", frameErr)
	}
}

// TestTinkerPingTimeout proves a wedged shim does not hang the CLI, and that
// Close still tears it down (covering the Close error branch when `go run`
// reports the kill).
func TestTinkerPingTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a `go run` subprocess")
	}

	dir := t.TempDir()
	writeZeverFixture(t, dir, "go.mod", "module sleepshim\n\ngo 1.24\n")
	// The sleeper blocks reading stdin: closing stdin (as Close does) ends
	// it promptly instead of leaving `go run` wedged.
	writeZeverFixture(t, dir, "main.go", "package main\n\nimport (\n\t\"io\"\n\t\"os\"\n)\n\nfunc main() { _, _ = io.ReadAll(os.Stdin) }\n")
	t.Chdir(dir)

	client, err := startTinkerShim(".", nil)
	if err != nil {
		t.Fatalf("startTinkerShim: %v", err)
	}

	if err := client.ping(100 * time.Millisecond); err == nil {
		_ = client.Close()
		t.Fatal("expected a ping timeout, got nil")
	} else if !strings.Contains(err.Error(), "did not respond") {
		_ = client.Close()
		t.Fatalf("expected timeout error, got: %v", err)
	}

	// Close must terminate the sleeper; `go run` exits non-zero on kill, so
	// Close reports (and wraps) that exit.
	if err := client.Close(); err == nil {
		t.Fatal("expected Close to report the killed shim, got nil")
	}

	if err := client.Close(); err != nil {
		t.Fatalf("second Close must be a no-op, got: %v", err)
	}
}

// TestRunTinkerHelp proves the help path never touches the shim.
func TestRunTinkerHelp(t *testing.T) {
	if err := runTinker([]string{"-h"}); err != nil {
		t.Fatalf("runTinker -h: %v", err)
	}
}

// TestRunTinkerBadFlag proves flag errors surface instead of reaching the shim.
func TestRunTinkerBadFlag(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := runTinker([]string{"--bogus-flag"}); err == nil {
		t.Fatal("expected a flag error, got nil")
	}
}

// TestRunTinkerExplicitMissingEntry pins --entry resolution through the config.
func TestRunTinkerExplicitMissingEntry(t *testing.T) {
	t.Chdir(t.TempDir())

	cfg := resolveTinkerConfig(ProjectConfig{}.withDefaults(), "custom/shim")
	if cfg.Entry != "custom/shim" {
		t.Fatalf("Entry = %q, want custom/shim", cfg.Entry)
	}

	if err := runTinker([]string{"--entry", "custom/shim"}); err == nil {
		t.Fatal("expected missing-shim error, got nil")
	}
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

// TestTinkerREPLReadError proves an input failure surfaces as an error.
func TestTinkerREPLReadError(t *testing.T) {
	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	want := errTestSentinel
	if err := tinkerREPL(i, errReader{err: want}, &strings.Builder{}); err == nil {
		t.Fatal("expected a read error, got nil")
	}
}

// TestUnwrapTinkerValue pins the normalization branches directly.
func TestUnwrapTinkerValue(t *testing.T) {
	t.Parallel()

	if got := unwrapTinkerValue(reflect.Value{}); got.IsValid() {
		t.Fatal("invalid value must stay invalid")
	}

	type str struct{ S string }

	// A pointer to a non-interface (a struct) is a real value, not yaegi's
	// *interface{} wrapper: returned untouched.
	v := reflect.ValueOf(&str{S: "x"})
	if got := unwrapTinkerValue(v); got != v {
		t.Fatal("pointer to struct must be returned untouched")
	}

	// A nil interface is returned as-is: there is nothing to unwrap to.
	var z any
	if got := unwrapTinkerValue(reflect.ValueOf(&z).Elem()); !got.IsValid() || !got.IsNil() {
		t.Fatal("nil interface must be returned as-is")
	}

	// A populated *interface{} unwraps to the concrete value.
	x := any("concrete")
	if got := formatTinkerValue(reflect.ValueOf(&x)); got != "concrete" {
		t.Fatalf("unwrapped value = %q, want concrete", got)
	}

	if got := formatTinkerValue(reflect.Value{}); got != "" {
		t.Fatalf("invalid value formats as %q, want empty", got)
	}
}

// TestTinkerClosedClientErrors proves every container method fails fast on a
// closed client instead of blocking or panicking.
func TestTinkerClosedClientErrors(t *testing.T) {
	t.Parallel()

	newClosed := func() *tinkerClient {
		c := &tinkerClient{}
		c.closed.Store(true)
		return c
	}

	c := newClosed()

	if _, err := (&tinkerDB{c: c}).Query("select 1"); err == nil {
		t.Error("Query on closed client = nil, want error")
	}

	if err := (&tinkerDB{c: c}).Exec("select 1"); err == nil {
		t.Error("Exec on closed client = nil, want error")
	}

	if _, err := (&tinkerCache{c: c}).Get("k"); err == nil {
		t.Error("Get on closed client = nil, want error")
	}

	if err := (&tinkerCache{c: c}).Set("k", "v", 0); err == nil {
		t.Error("Set on closed client = nil, want error")
	}

	if err := (&tinkerCache{c: c}).Delete("k"); err == nil {
		t.Error("Delete on closed client = nil, want error")
	}

	if _, err := (&tinkerCache{c: c}).Exists("k"); err == nil {
		t.Error("Exists on closed client = nil, want error")
	}

	if err := (&tinkerQueue{c: c}).Push("t", "p"); err == nil {
		t.Error("Push on closed client = nil, want error")
	}

	if _, err := (&tinkerQueue{c: c}).Length("t"); err == nil {
		t.Error("Length on closed client = nil, want error")
	}

	if err := (&tinkerJob{c: c}).Dispatch("j", nil); err == nil {
		t.Error("Dispatch on closed client = nil, want error")
	}

	if err := (&tinkerClient{}).Close(); err != nil {
		t.Fatalf("Close on zero client: %v", err)
	}

	if err := newClosed().ping(time.Second); err == nil {
		t.Error("ping on closed client = nil, want error")
	}
}

// TestTinkerCallErrors pins the request-encoding and transport error
// branches with piped fakes instead of a subprocess.
func TestTinkerCallErrors(t *testing.T) {
	t.Parallel()

	// Unmarshallable args fail before any write.
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	defer func() { _ = r.Close() }()

	c := &tinkerClient{stdin: w, stdout: bufio.NewReader(r)}

	if _, callErr := c.call(verbPing, func() {}); callErr == nil {
		t.Error("call with unmarshallable args = nil, want error")
	}

	// A closed stdin fails the write.
	_ = w.Close()

	if _, callErr := c.call(verbPing, nil); callErr == nil {
		t.Error("call on closed stdin = nil, want error")
	}

	// EOF on stdout fails the read.
	r2, w2, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	_ = w2.Close()

	c2 := &tinkerClient{stdin: w, stdout: bufio.NewReader(r2)}

	defer func() { _ = r2.Close() }()

	if _, err := c2.call(verbPing, nil); err == nil {
		t.Error("call on EOF stdout = nil, want error")
	}
}

// TestTinkerCallIntoErrors pins the result-decoding branches with scripted
// framed responses.
func TestTinkerCallIntoErrors(t *testing.T) {
	t.Parallel()

	scripted := func(line string) *tinkerClient {
		t.Helper()

		r, w, err := os.Pipe()
		if err != nil {
			t.Fatalf("pipe: %v", err)
		}

		t.Cleanup(func() { _ = r.Close() })

		return &tinkerClient{stdin: w, stdout: bufio.NewReader(strings.NewReader(line))}
	}

	// Empty result with a non-nil destination.
	c := scripted("@@zever-tinker@@{}\n")
	defer func() { _ = c.stdin.Close() }()

	var out cacheGetResult
	if err := c.callInto(verbCacheGet, cacheGetArgs{Key: "k"}, &out); err == nil {
		t.Error("callInto with empty result = nil, want error")
	}

	// Result shape mismatching the destination.
	c2 := scripted("@@zever-tinker@@{\"result\":{\"value\":\"hello\"}}\n")
	defer func() { _ = c2.stdin.Close() }()

	var n int64Result
	if err := c2.callInto(verbCacheGet, cacheGetArgs{Key: "k"}, &n); err == nil {
		t.Error("callInto with mismatched shape = nil, want error")
	}

	// Unmarshallable job args fail before any write.
	if err := (&tinkerJob{c: c}).Dispatch("j", func() {}); err == nil {
		t.Error("Dispatch with unmarshallable args = nil, want error")
	}
}

// TestTinkerAllowedSymbolsMatchesUpstream pins the denylist against whatever
// yaegi actually ships: every blocked import path present upstream must be
// absent downstream.
func TestTinkerAllowedSymbolsMatchesUpstream(t *testing.T) {
	t.Parallel()

	allowed := tinkerAllowedSymbols()

	for key := range stdlib.Symbols {
		pkg := symKeyImportPath(key)

		for _, blocked := range tinkerBlockedImports {
			if pkg == blocked && allowed[key] != nil {
				t.Fatalf("blocked package %q present in allowlist (key %q)", pkg, key)
			}
		}
	}
}

// TestFormatTinkerValueUnaddressable pins the no-interface branch with an
// unexported struct field value.
func TestFormatTinkerValueUnaddressable(t *testing.T) {
	t.Parallel()

	type hidden struct {
		s string
	}

	v := reflect.ValueOf(hidden{s: "x"}).Field(0)

	if v.CanInterface() {
		t.Skip("field is interfaceable on this toolchain")
	}

	if got := formatTinkerValue(v); got == "" {
		t.Fatal("unaddressable value must still render")
	}
}

// TestRunTinkerInteractiveFlag pins the -i global wiring on the error path.
func TestRunTinkerInteractiveFlag(t *testing.T) {
	t.Chdir(t.TempDir())

	interactiveMode = false
	t.Cleanup(func() { interactiveMode = false })

	if err := runTinker([]string{"-i"}); err == nil {
		t.Fatal("expected missing-shim error, got nil")
	}

	if !interactiveMode {
		t.Fatal("-i must set interactiveMode")
	}
}

// TestRunTinkerShimStartFailure pins the spawn error branch: the entry
// exists, but there is no go toolchain to run it with.
func TestRunTinkerShimStartFailure(t *testing.T) {
	dir := t.TempDir()
	shimDir := filepath.Join(dir, "shim")

	if err := os.MkdirAll(shimDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	t.Chdir(dir)
	t.Setenv("PATH", "")

	if err := runTinker([]string{"--entry", "shim"}); err == nil {
		t.Fatal("expected a shim start error, got nil")
	}
}

// TestRunTinkerDashHelpFlag exercises the flag package's own -help path,
// which routes through the Usage closure.
func TestRunTinkerDashHelpFlag(t *testing.T) {
	if err := runTinker([]string{"-help"}); err != nil {
		t.Fatalf("runTinker -help: %v", err)
	}
}

// TestTinkerCloseWaitsCleanExit proves Close returns nil when the child has
// already exited cleanly: no kill, just a reap. (Closing around a live `go
// run` shim instead races the teardown kill against the child's own exit,
// so that path only asserts prompt return elsewhere.)
func TestTinkerCloseWaitsCleanExit(t *testing.T) {
	t.Parallel()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	defer func() { _ = r.Close() }()

	cmd := exec.CommandContext(t.Context(), "true")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start true: %v", err)
	}

	c := &tinkerClient{cmd: cmd, stdin: w}

	if err := c.Close(); err != nil {
		t.Fatalf("Close on cleanly exited child: %v", err)
	}
}

// TestTinkerReadPartialLine pins the passthrough of a non-newline-terminated
// fragment when the shim dies mid-line.
func TestTinkerReadPartialLine(t *testing.T) {
	t.Parallel()

	var passthrough strings.Builder

	c := &tinkerClient{
		stdout:      bufio.NewReader(strings.NewReader("@@zever-tinker@@partial")),
		passthrough: &passthrough,
	}

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	defer func() { _ = r.Close() }()

	c.stdin = w

	defer func() { _ = w.Close() }()

	if _, err := c.call(verbPing, nil); err == nil {
		t.Fatal("expected a read error, got nil")
	}

	if !strings.Contains(passthrough.String(), "partial") {
		t.Fatalf("expected the fragment to be forwarded, got %q", passthrough.String())
	}
}

// TestTinkerREPLEOF proves clean EOF without :exit returns nil.
func TestTinkerREPLEOF(t *testing.T) {
	i, err := newTinkerInterp(&tinkerClient{})
	if err != nil {
		t.Fatalf("newTinkerInterp: %v", err)
	}

	var out strings.Builder

	if err := tinkerREPL(i, strings.NewReader("1 + 2\n"), &out); err != nil {
		t.Fatalf("tinkerREPL: %v", err)
	}

	if !strings.Contains(out.String(), "3") {
		t.Fatalf("expected evaluation output, got:\n%s", out.String())
	}
}

// TestRunTinkerBadProject pins the config error branch of runTinker.
func TestRunTinkerBadProject(t *testing.T) {
	dir := t.TempDir()
	writeZeverFixture(t, dir, "zever.yaml", "project:\n  tinker_entry: [unclosed\n")
	t.Chdir(dir)

	if err := runTinker(nil); err == nil {
		t.Fatal("expected a config error, got nil")
	}
}
