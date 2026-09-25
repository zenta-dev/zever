package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"go/parser"
	"io"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// tinkerBindPkg is the synthetic package the shim-backed values are exported
// under before the REPL prelude re-binds them to lowercase globals. yaegi's
// Use() takes a "<import path>/<package name>" key.
const tinkerBindPkg = "zevertinker/zevertinker"

// tinkerPrelude runs before the user's first line. It lifts the injected
// values into the short, Rails-console-ish names the REPL documents.
//
// Each line is evaluated on its own: yaegi parses a multi-line Eval as a file
// body, where a bare `x := y` is not a declaration.
var tinkerPrelude = []string{
	// A REPL is unusable without these; yaegi imports nothing by default.
	`import ("fmt"; "strings"; "strconv"; "time"; "encoding/json")`,
	`import "zevertinker"`,
	`db := zevertinker.DB`,
	`cache := zevertinker.Cache`,
	`queue := zevertinker.Queue`,
	`job := zevertinker.Job`,
}

// tinkerDevWarning is printed to stderr on every start. The REPL evaluates
// arbitrary Go with live container credentials: it is a development-only
// tool and must never run against production data.
const tinkerDevWarning = "WARNING: zever tinker is development-only: it evaluates arbitrary Go against a live container, so run it against a development database only."

func styledTinkerBanner() string {
	header := title("zever tinker") + dim(" — live container REPL (yaegi)")

	body := joinLines(
		header,
		"",
		dim("  ")+cmd(`db.Query("select 1")`)+dim("            ")+cmd(`db.Exec("insert into ...", args...)`),
		dim("  ")+cmd(`cache.Set("k", "v", 60)`)+dim("         ")+cmd(`cache.Get("k") / cache.Delete("k") / cache.Exists("k")`),
		dim("  ")+cmd(`queue.Push("topic", "payload")`)+dim("  ")+cmd(`queue.Length("topic")`),
		dim("  ")+cmd(`job.Dispatch("name", args)`),
		"",
		dim("Go expressions are evaluated by yaegi with the standard library available."),
		dim("Type ")+cmd(":help")+dim(" for this banner, ")+cmd(":exit")+dim(" (or Ctrl-D) to quit."),
	)
	if colorEnabled {
		return box(body)
	}

	return body
}

func printTinkerUsage(fs *flag.FlagSet) {
	header := title("zever tinker") + dim(" — live container REPL")
	usage := bold("Usage:") + "  " + cmd("zever tinker") + dim(" [--entry PATH]") + dim("  •  -i for guided prompts")

	body := joinLines(
		header,
		"",
		usage,
		"",
		bold("Flags:"),
	)
	if colorEnabled {
		_, _ = fmt.Fprintln(fs.Output(), box(body))
	} else {
		_, _ = fmt.Fprintln(fs.Output(), body)
	}

	fs.PrintDefaults()
	_, _ = fmt.Fprintln(fs.Output(), "")
	_, _ = fmt.Fprintln(fs.Output(), dim("Examples:"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever tinker"))
	_, _ = fmt.Fprintln(fs.Output(), dim("  ")+cmd("zever tinker --entry ./cmd/tinker-shim"))

	if shouldShowHint() {
		_, _ = fmt.Fprintln(fs.Output(), formatHint("run 'zever generate tinker' to scaffold the shim first"))
	}
}

// tinkerStartTimeout bounds the readiness ping. The shim is spawned with
// `go run`, so the first response waits on a compile.
const tinkerStartTimeout = 3 * time.Minute

// tinkerEvalTimeout bounds one REPL evaluation. Interpreted code runs in the
// CLI process with live container values in scope; without a bound, an
// accidental infinite loop would hang the session forever.
var tinkerEvalTimeout = 30 * time.Second

// TinkerConfig is the resolved configuration for one `zever tinker` session.
type TinkerConfig struct {
	// Entry is the shim package directory to run via `go run`.
	Entry string
	// StartTimeout bounds the shim readiness ping.
	StartTimeout time.Duration
	// EvalTimeout bounds one REPL evaluation.
	EvalTimeout time.Duration
}

// resolveTinkerConfig builds a TinkerConfig from an explicit --entry flag and
// the project layout without side effects.
func resolveTinkerConfig(project ProjectConfig, entryFlag string) TinkerConfig {
	entry := strings.TrimSpace(entryFlag)
	if entry == "" {
		entry = project.TinkerEntry
	}

	return TinkerConfig{Entry: entry, StartTimeout: tinkerStartTimeout, EvalTimeout: tinkerEvalTimeout}
}

// runTinker starts the project's tinker shim and drives a yaegi REPL whose
// db/cache/queue/job values proxy to it. See tinker_protocol.go for why the
// container lives in a subprocess.
func runTinker(args []string) error {
	if hasHelpFlag(args) {
		fs := flag.NewFlagSet("tinker", flag.ContinueOnError)
		// coverageProof: no fs.Usage closure here. This FlagSet is never
		// parsed (printTinkerUsage is called directly below), so a Usage
		// closure would be unreachable; the parsed FlagSet below owns one.
		printTinkerUsage(fs)

		return nil
	}

	args = peelInteractive(args)
	fs := flag.NewFlagSet("tinker", flag.ContinueOnError)
	entry := fs.String("entry", "", "shim package to run (default: the project config's tinker_entry)")
	// coverageProof: no local -i/--interactive flags (cf. db.go's removed
	// arm). peelInteractive strips them before Parse and sets interactiveMode
	// globally, so the FlagSet could never see them.
	fs.Usage = func() { printTinkerUsage(fs) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}

		return err
	}

	pc, err := loadProjectConfig()
	if err != nil {
		return err
	}

	cfg := resolveTinkerConfig(pc, *entry)

	if _, serr := os.Stat(cfg.Entry); os.IsNotExist(serr) {
		return fmt.Errorf("zever tinker: no tinker shim at %s — run 'zever generate tinker' first", cfg.Entry)
	}

	client, err := startTinkerShim(cfg.Entry, os.Stderr)
	if err != nil {
		return err
	}

	defer func() { _ = client.Close() }()

	if pingErr := client.ping(cfg.StartTimeout); pingErr != nil {
		// Tear down the wedged shim instead of leaking it: Close cancels
		// the context, which unblocks ping's orphaned read.
		_ = client.Close()
		return pingErr
	}

	i, err := newTinkerInterp(client)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintln(os.Stderr, yellow(tinkerDevWarning))
	_, _ = fmt.Fprintln(os.Stdout, styledTinkerBanner())

	return tinkerREPL(i, os.Stdin, os.Stdout)
}

// tinkerBlockedImports is the denylist subtracted from yaegi's stdlib symbol
// table before it is handed to Use. The dirty code passed stdlib.Symbols
// wholesale; that table ships syscall and unsafe wrappers (and would ship
// os/exec if yaegi ever extracts it), each of which lets interpreted code
// escape the shim protocol: arbitrary host processes, raw syscalls, pointer
// games. The REPL's whole container surface is the four shim-backed values;
// it needs no process control and no unsafe.
var tinkerBlockedImports = []string{"os/exec", "syscall", "unsafe", "plugin"}

// tinkerAllowedSymbols returns yaegi's stdlib symbols minus tinkerBlockedImports.
// The map is copied: stdlib.Symbols itself is never mutated.
func tinkerAllowedSymbols() map[string]map[string]reflect.Value {
	out := make(map[string]map[string]reflect.Value, len(stdlib.Symbols))

outer:
	for key, pkg := range stdlib.Symbols {
		for _, blocked := range tinkerBlockedImports {
			if symKeyImportPath(key) == blocked {
				continue outer
			}
		}

		out[key] = pkg
	}

	return out
}

// symKeyImportPath recovers the import path from a yaegi symbol-table key,
// which has the form "<import path>/<package name>" (e.g. "os/exec/os_exec").
func symKeyImportPath(key string) string {
	if i := strings.LastIndex(key, "/"); i >= 0 {
		return key[:i]
	}

	return key
}

// newTinkerInterp builds a yaegi interpreter with the curated standard
// library plus the four shim-backed values, already bound to their lowercase
// REPL names.
func newTinkerInterp(client *tinkerClient) (*interp.Interpreter, error) {
	i := interp.New(interp.Options{})

	if err := i.Use(tinkerAllowedSymbols()); err != nil {
		return nil, fmt.Errorf("zever tinker: load stdlib symbols: %w", err)
	}

	exports := interp.Exports{
		tinkerBindPkg: {
			"DB":    reflect.ValueOf(&tinkerDB{c: client}),
			"Cache": reflect.ValueOf(&tinkerCache{c: client}),
			"Queue": reflect.ValueOf(&tinkerQueue{c: client}),
			"Job":   reflect.ValueOf(&tinkerJob{c: client}),
		},
	}

	if err := i.Use(exports); err != nil {
		return nil, fmt.Errorf("zever tinker: bind container values: %w", err)
	}

	for _, line := range tinkerPrelude {
		ctx, cancel := context.WithTimeout(context.Background(), tinkerEvalTimeout)
		_, err := i.EvalWithContext(ctx, line)
		cancel()

		if err != nil {
			return nil, fmt.Errorf("zever tinker: bind REPL prelude (%s): %w", line, err)
		}
	}

	return i, nil
}

// tinkerREPL reads one Go expression or statement per line and prints its
// value. Evaluation errors are reported and the loop continues — a typo must
// not cost the developer their session (and the shim's container with it).
func tinkerREPL(i *interp.Interpreter, in io.Reader, out io.Writer) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for {
		_, _ = fmt.Fprint(out, cyan("zever> "))

		if !sc.Scan() {
			break
		}

		line := strings.TrimSpace(sc.Text())

		switch line {
		case "":
			continue
		case ":exit", ":quit", "exit", "quit":
			return nil
		case ":help":
			_, _ = fmt.Fprintln(out, styledTinkerBanner())
			continue
		}

		v, err := tinkerEval(i, line)
		if err != nil {
			_, _ = fmt.Fprintf(out, "%s %s\n", failMark(), red(fmt.Sprintf("error: %v", err)))
			continue
		}

		if s := formatTinkerValue(v); s != "" {
			_, _ = fmt.Fprintln(out, s)
		}
	}

	_, _ = fmt.Fprintln(out)

	if err := sc.Err(); err != nil {
		return fmt.Errorf("zever tinker: read input: %w", err)
	}

	return nil
}

// tinkerEval evaluates one line of REPL input with a timeout. Every path that
// feeds the interpreter goes through here so the yaegi workaround in
// normalizeTinkerSource and the timeout apply uniformly.
func tinkerEval(i *interp.Interpreter, src string) (reflect.Value, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tinkerEvalTimeout)
	defer cancel()

	v, err := i.EvalWithContext(ctx, normalizeTinkerSource(src))
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return v, fmt.Errorf("zever tinker: evaluation timed out after %s: %w", tinkerEvalTimeout, err)
		}

		return v, err
	}

	return v, nil
}

// normalizeTinkerSource works around a yaegi 0.16.1 bug that silently
// discards the value of an immediately-invoked function literal.
//
// Interpreter.parse switches on the source's first token: for token.FUNC it
// assumes a declaration and prefixes "package main;" instead of wrapping the
// source in a main function body. `func() string { … }()` is an expression, so
// that parse fails, and the retry path re-wraps the source in a main function
// but never sets the parser's inFunc flag — so yaegi compiles the line as a
// file declaring main rather than as an expression, and Eval hands back an
// unpopulated *interface{} instead of the call's result.
//
// Parenthesising the literal makes the first token a '(' , which takes the
// ordinary statement path and returns the real value. Only sources that are
// whole Go expressions starting with the func keyword are rewritten, so
// genuine declarations (`func f() {}`) and every other line are untouched.
func normalizeTinkerSource(src string) string {
	trimmed := strings.TrimSpace(src)

	if !strings.HasPrefix(trimmed, "func") {
		return src
	}

	// Guard against identifiers that merely begin with "func", e.g. `funcs`.
	if rest := trimmed[len("func"):]; rest != "" && isTinkerIdentRune(rest[0]) {
		return src
	}

	if _, err := parser.ParseExpr(trimmed); err != nil {
		return src
	}

	return "(" + trimmed + ")"
}

// isTinkerIdentRune reports whether b can continue a Go identifier.
func isTinkerIdentRune(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9', b == '_':
		return true
	default:
		return false
	}
}

// unwrapTinkerValue normalizes an Eval result. yaegi hands back the result of
// an interpreted function call as a *interface{} pointing at the real value,
// which is an implementation detail neither the REPL nor its tests should
// have to see.
func unwrapTinkerValue(v reflect.Value) reflect.Value {
	for v.IsValid() && (v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface) {
		if v.Kind() == reflect.Pointer && v.Type().Elem().Kind() != reflect.Interface {
			return v
		}

		if v.IsNil() {
			return v
		}

		v = v.Elem()
	}

	return v
}

// formatTinkerValue renders an Eval result. Statements yield an invalid
// reflect.Value, which prints nothing.
func formatTinkerValue(v reflect.Value) string {
	v = unwrapTinkerValue(v)

	if !v.IsValid() {
		return ""
	}

	if !v.CanInterface() {
		return v.String()
	}

	return fmt.Sprintf("%v", v.Interface())
}

// --- shim client ------------------------------------------------------

// tinkerClient owns the pipe conversation with one shim subprocess: it
// marshals a tinkerRequest per call, writes it as one line, and reads back
// the next framed response line. Calls are serialized because the protocol
// has no request ids — one request, one response, in order.
type tinkerClient struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stdin  io.WriteCloser
	stdout *bufio.Reader
	// passthrough receives any shim stdout line that is not protocol
	// traffic, e.g. output from the project's own logger.
	passthrough io.Writer

	mu     sync.Mutex
	closed atomic.Bool
}

// startTinkerShim spawns `go run <target>` with piped stdin/stdout. Unlike
// the launcher subcommands, tinker cannot inherit stdio: it owns the pipe
// conversation. The shim's stderr is inherited so build errors and app logs
// reach the developer directly.
func startTinkerShim(target string, passthrough io.Writer) (*tinkerClient, error) {
	pkg := target
	if !strings.HasPrefix(pkg, "./") && !strings.HasPrefix(pkg, "../") {
		pkg = "./" + pkg
	}

	// The shim lives as long as the client does; Close cancels the context so
	// a shim that ignores its stdin closing is still torn down.
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, "go", "run", pkg) //nolint:gosec // target is a developer-supplied package path
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("zever tinker: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("zever tinker: stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("zever tinker: start shim %q: %w", pkg, err)
	}

	if passthrough == nil {
		passthrough = io.Discard
	}

	return &tinkerClient{
		cmd:         cmd,
		cancel:      cancel,
		stdin:       stdin,
		stdout:      bufio.NewReaderSize(stdout, 64*1024),
		passthrough: passthrough,
	}, nil
}

// Close shuts the shim down. Order matters: stdin closes before the call
// mutex is taken, and the context is cancelled before Wait. A call wedged in
// readResponse (e.g. after a ping timeout) holds the mutex while blocked on
// the shim's stdout, whose write end is inherited by the shim's own child;
// that child only exits once it observes stdin EOF, so locking first would
// deadlock: Close waiting on the mutex, the orphan waiting on stdout, the
// shim waiting on stdin, stdin waiting on the mutex. Closing twice is a no-op.
func (c *tinkerClient) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	// Unblock the shim first (lock-free: idempotent via the swap above, and
	// an in-flight write simply fails with ErrClosed on a closing client).
	if c.stdin != nil {
		_ = c.stdin.Close()
	}

	// Then kill the supervisor so a shim ignoring stdin EOF still goes down
	// and the orphaned read observes EOF and releases the mutex.
	if c.cancel != nil {
		c.cancel()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd == nil {
		return nil
	}

	if err := c.cmd.Wait(); err != nil {
		return fmt.Errorf("zever tinker: shim exited: %w", err)
	}

	return nil
}

// ping waits for the shim to become ready, bounded by timeout — `go run`
// compiles the project before the shim's first byte, which is not fast.
func (c *tinkerClient) ping(timeout time.Duration) error {
	done := make(chan error, 1)

	// Ping probe exits after call completes (buffered, no leak on timeout).
	go func() {
		_, err := c.call(verbPing, nil)
		done <- err
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("zever tinker: shim did not start: %w", err)
		}

		return nil
	case <-time.After(timeout):
		return fmt.Errorf("zever tinker: shim did not respond within %s", timeout)
	}
}

// call performs one request/response round trip.
func (c *tinkerClient) call(verb string, args any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed.Load() {
		return nil, errors.New("zever tinker: shim is closed")
	}

	if c.stdin == nil || c.stdout == nil {
		return nil, errors.New("zever tinker: shim is not connected")
	}

	req := tinkerRequest{Verb: verb}

	if args != nil {
		raw, err := json.Marshal(args)
		if err != nil {
			return nil, fmt.Errorf("zever tinker: marshal %s args: %w", verb, err)
		}

		req.Args = raw
	}

	line, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("zever tinker: marshal %s request: %w", verb, err)
	}

	if _, writeErr := c.stdin.Write(append(line, '\n')); writeErr != nil {
		return nil, fmt.Errorf("zever tinker: write %s request: %w", verb, writeErr)
	}

	resp, err := c.readResponse()
	if err != nil {
		return nil, err
	}

	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}

	return resp.Result, nil
}

// readResponse consumes shim stdout until a framed protocol line arrives,
// forwarding everything else to the passthrough writer.
func (c *tinkerClient) readResponse() (tinkerResponse, error) {
	for {
		line, err := c.stdout.ReadString('\n')
		if err != nil {
			if len(strings.TrimSpace(line)) > 0 {
				_, _ = fmt.Fprintln(c.passthrough, line)
			}

			return tinkerResponse{}, fmt.Errorf("zever tinker: read shim response: %w", err)
		}

		line = strings.TrimRight(line, "\r\n")

		payload, ok := strings.CutPrefix(line, tinkerFramePrefix)
		if !ok {
			_, _ = fmt.Fprintln(c.passthrough, line)
			continue
		}

		var resp tinkerResponse
		if err := json.Unmarshal([]byte(payload), &resp); err != nil {
			return tinkerResponse{}, fmt.Errorf("zever tinker: decode shim response: %w", err)
		}

		return resp, nil
	}
}

// callInto performs a round trip and decodes the result into dst.
func (c *tinkerClient) callInto(verb string, args, dst any) error {
	raw, err := c.call(verb, args)
	if err != nil {
		return err
	}

	if dst == nil {
		return nil
	}

	if len(raw) == 0 {
		return fmt.Errorf("zever tinker: %s returned no result", verb)
	}

	if err := json.Unmarshal(raw, dst); err != nil {
		return fmt.Errorf("zever tinker: decode %s result: %w", verb, err)
	}

	return nil
}

// --- REPL-facing values ----------------------------------------------
//
// These are the four values bound into the interpreter. Their methods are the
// whole of the REPL's container surface: deliberately narrow, one method per
// protocol verb, so that no part of it depends on a project's generated code.

// tinkerDB is the REPL's `db`.
type tinkerDB struct{ c *tinkerClient }

// Query runs a query and returns one map per row, keyed by column name.
func (d *tinkerDB) Query(sql string, args ...any) ([]map[string]any, error) {
	var res dbQueryResult
	if err := d.c.callInto(verbDBQuery, dbQueryArgs{SQL: sql, Args: args}, &res); err != nil {
		return nil, err
	}

	out := make([]map[string]any, 0, len(res.Rows))

	for _, row := range res.Rows {
		m := make(map[string]any, len(res.Columns))

		for i, col := range res.Columns {
			if i < len(row) {
				m[col] = row[i]
			}
		}

		out = append(out, m)
	}

	return out, nil
}

// Exec runs a statement that returns no rows.
func (d *tinkerDB) Exec(sql string, args ...any) error {
	return d.c.callInto(verbDBExec, dbExecArgs{SQL: sql, Args: args}, nil)
}

// tinkerCache is the REPL's `cache`.
type tinkerCache struct{ c *tinkerClient }

// Get returns the cached value for key as a string.
func (t *tinkerCache) Get(key string) (string, error) {
	var res cacheGetResult
	if err := t.c.callInto(verbCacheGet, cacheGetArgs{Key: key}, &res); err != nil {
		return "", err
	}

	return res.Value, nil
}

// Set stores value under key. A ttlSeconds of zero or less means no expiry.
func (t *tinkerCache) Set(key, value string, ttlSeconds int) error {
	return t.c.callInto(verbCacheSet, cacheSetArgs{Key: key, Value: value, TTLSeconds: ttlSeconds}, nil)
}

// Delete removes key.
func (t *tinkerCache) Delete(key string) error {
	return t.c.callInto(verbCacheDelete, cacheDeleteArgs{Key: key}, nil)
}

// Exists reports whether key is present.
func (t *tinkerCache) Exists(key string) (bool, error) {
	var res boolResult
	if err := t.c.callInto(verbCacheExists, cacheExistsArgs{Key: key}, &res); err != nil {
		return false, err
	}

	return res.Value, nil
}

// tinkerQueue is the REPL's `queue`.
type tinkerQueue struct{ c *tinkerClient }

// Push enqueues payload on topic.
func (t *tinkerQueue) Push(topic, payload string) error {
	return t.c.callInto(verbQueuePush, queuePushArgs{Topic: topic, Payload: payload}, nil)
}

// Length reports how many ready messages topic holds.
func (t *tinkerQueue) Length(topic string) (int64, error) {
	var res int64Result
	if err := t.c.callInto(verbQueueLength, queueLengthArgs{Topic: topic}, &res); err != nil {
		return 0, err
	}

	return res.Value, nil
}

// tinkerJob is the REPL's `job`.
type tinkerJob struct{ c *tinkerClient }

// Dispatch enqueues a registered job. args is JSON-encoded on the way over.
func (t *tinkerJob) Dispatch(name string, args any) error {
	raw, err := json.Marshal(args)
	if err != nil {
		return fmt.Errorf("zever tinker: marshal job args: %w", err)
	}

	return t.c.callInto(verbJobDispatch, jobDispatchArgs{Name: name, Args: raw}, nil)
}
