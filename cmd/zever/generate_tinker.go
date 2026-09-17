package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"go/format"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

const tinkerGenerateUsageBody = `Scaffolds the per-project tinker shim: a ` + "`package main`" + ` program compiled inside the
target project's module that builds the real container via its internal/app package
and answers ` + "`zever tinker`" + `'s line-delimited JSON verbs over stdin/stdout.

The shim exists because ` + "`zever`" + `, a root-module binary, cannot import a
project's internal/app across the module boundary.`

// coverageSeam: prompt indirection for tests. Proof: huh requires a real TTY;
// var seams keep behavior identical while enabling success/error coverage.
//
//nolint:dupl
var (
	promptInputForTinkerGen   = promptInput
	promptConfirmForTinkerGen = promptConfirm
)

func printTinkerGenerateUsage(fs *flag.FlagSet) {
	header := title("zever generate tinker") + dim(" — scaffold tinker shim")
	usage := bold("Usage:") + "  " + cmd("zever generate tinker") + dim(" [--app PATH] [--dir DIR] [--force]") + dim("  •  -i/--interactive")

	printBoxedUsage(fs, header, usage, tinkerGenerateUsageBody, []usageExample{
		{command: "zever generate tinker"},
		{command: "zever generate tinker --app example.com/app/internal/app --dir ./tinker/shim"},
		{command: "zever generate tinker -i", comment: "  # guided: app package & output dir"},
	}, "then 'zever tinker' to start the REPL")
}

// GenerateTinkerConfig is the pure input to GenerateTinker: the app package
// import path, the output directory, and the force flag.
type GenerateTinkerConfig struct {
	AppPackage string
	OutDir     string
	Force      bool
	Stdout     io.Writer
	Stderr     io.Writer
}

// GenerateTinker renders the tinker shim and writes it to
// <OutDir>/main.go, refusing to clobber an existing shim unless Force is
// set. It performs no flag parsing and no prompting, and returns the shim
// path.
func GenerateTinker(cfg GenerateTinkerConfig) (string, error) {
	const tag = "zever generate tinker"

	if strings.TrimSpace(cfg.AppPackage) == "" {
		return "", fmt.Errorf("%s: app package must not be empty (pass --app)", tag)
	}

	if strings.TrimSpace(cfg.OutDir) == "" {
		return "", fmt.Errorf("%s: output directory must not be empty (pass --dir)", tag)
	}

	if isTraversalName(cfg.OutDir) && (cfg.OutDir == ".." || filepath.IsAbs(cfg.OutDir)) {
		return "", fmt.Errorf("%s: %w: %q", tag, ErrPathTraversal, cfg.OutDir)
	}

	src, err := renderTinkerShim(cfg.AppPackage, cfg.OutDir)
	if err != nil {
		return "", err
	}

	path := filepath.Join(cfg.OutDir, "main.go")

	if !cfg.Force {
		if _, serr := os.Stat(path); serr == nil {
			return "", fmt.Errorf("%s: %s already exists (pass --force to overwrite)", tag, path)
		} else if !os.IsNotExist(serr) {
			return "", fmt.Errorf("%s: stat %q: %w", tag, path, serr)
		}
	}

	if err := os.MkdirAll(cfg.OutDir, 0o750); err != nil {
		return "", fmt.Errorf("%s: mkdir %q: %w", tag, cfg.OutDir, err)
	}

	if err := os.WriteFile(path, src, 0o644); err != nil { //nolint:gosec // generated scaffold, not a secret
		return "", fmt.Errorf("%s: write %q: %w", tag, path, err)
	}

	if stdout := cfg.Stdout; stdout != nil {
		_, _ = fmt.Fprintf(stdout, "%s %s %s %s %s\n", successMark(), success("scaffolded tinker shim at"), cyan(path), dim("(app package"), cyan(cfg.AppPackage)+dim(")"))
	}

	if stderr := cfg.Stderr; stderr != nil && shouldShowHint() {
		_, _ = fmt.Fprintln(stderr, formatHint("next: zever tinker"))
	}

	return path, nil
}

// runGenerateTinker scaffolds the per-project tinker shim: a `package main`
// program compiled inside the *target project's* module that builds the real
// container via its internal/app package and answers `zever tinker`'s
// line-delimited JSON verbs over stdin/stdout.
//
// The shim exists because `zever`, a root-module binary, cannot import a
// project's internal/app across the module boundary. See
// cmd/zever/tinker_protocol.go for the full rationale and the wire contract.
//
//nolint:gocyclo
func runGenerateTinker(args []string) error {
	args = peelInteractive(args)
	fs := flag.NewFlagSet("generate tinker", flag.ContinueOnError)
	appPkg := fs.String("app", "", "import path of the project's app package (default: <module>/internal/app)")
	dir := fs.String("dir", "", "output directory (default: the project config's tinker_entry)")
	force := fs.Bool("force", false, "overwrite an existing shim")
	interactive := fs.Bool("interactive", false, "prompt for missing values")
	interactiveShort := fs.Bool("i", false, "prompt for missing values (shorthand)")

	fs.Usage = func() { printTinkerGenerateUsage(fs) }

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *interactive || *interactiveShort {
		interactiveMode = true
	}

	pc, err := loadProjectConfig()
	if err != nil {
		return err
	}

	outDir := *dir
	if strings.TrimSpace(outDir) == "" {
		outDir = pc.TinkerEntry
		if isInteractiveTerminal() {
			val, perr := promptInputForTinkerGen("Output directory", outDir, nil)
			if perr != nil {
				return perr
			}

			if val != "" {
				outDir = val
			}
		}
	}

	pkg := *appPkg
	if strings.TrimSpace(pkg) == "" {
		mod, merr := currentModulePath()
		if merr != nil {
			if isInteractiveTerminal() {
				val, perr := promptInputForTinkerGen("App import path", "", func(s string) error {
					if strings.TrimSpace(s) == "" {
						return errors.New("must not be empty")
					}

					return nil
				})
				if perr != nil {
					return perr
				}

				pkg = val
			} else {
				return merr
			}
		} else {
			pkg = mod + "/internal/app"
			if isInteractiveTerminal() {
				val, perr := promptInputForTinkerGen("App import path", pkg, nil)
				if perr != nil {
					return perr
				}

				if val != "" {
					pkg = val
				}
			}
		}
	}

	forceVal := *force
	if !forceVal && isInteractiveTerminal() {
		if _, serr := os.Stat(filepath.Join(outDir, "main.go")); serr == nil {
			ok, perr := promptConfirmForTinkerGen(fmt.Sprintf("%q already exists — overwrite?", filepath.Join(outDir, "main.go")), "Yes, overwrite")
			if perr != nil {
				return perr
			}

			if ok {
				forceVal = true
			}
		}
	}

	_, err = GenerateTinker(GenerateTinkerConfig{
		AppPackage: pkg,
		OutDir:     outDir,
		Force:      forceVal,
		Stdout:     os.Stdout,
		Stderr:     os.Stderr,
	})

	//nolint:lll
	return err
}

// currentModulePath reads the module path from go.mod in the working
// directory. The shim needs a real import path for the project's app package
// and there is no other reliable way to guess it.
func currentModulePath() (string, error) {
	data, err := os.ReadFile("go.mod")
	if err != nil {
		return "", fmt.Errorf("zever generate tinker: read go.mod (run from the project root, or pass --app): %w", err)
	}

	for line := range strings.Lines(string(data)) {
		trimmed := strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(trimmed, "module"); ok {
			if path := strings.TrimSpace(rest); path != "" {
				return path, nil
			}
		}
	}

	return "", errors.New("zever generate tinker: go.mod has no module directive")
}

// renderTinkerShim renders the shim source for a project whose app package
// lives at appPkg and whose shim lives in entryDir, and gofmts it.
func renderTinkerShim(appPkg, entryDir string) ([]byte, error) {
	tmpl, err := template.New("tinker-shim").Parse(tinkerShimTemplate)
	if err != nil {
		return nil, fmt.Errorf("zever generate tinker: parse template: %w", err)
	}

	var buf bytes.Buffer

	data := struct {
		AppPkg      string
		EntryDir    string
		FramePrefix string
	}{AppPkg: appPkg, EntryDir: filepath.ToSlash(entryDir), FramePrefix: tinkerFramePrefix}

	if execErr := tmpl.Execute(&buf, data); execErr != nil {
		return nil, fmt.Errorf("zever generate tinker: render template: %w", execErr)
	}

	src, err := format.Source(buf.Bytes())
	if err != nil {
		return nil, fmt.Errorf("zever generate tinker: gofmt generated shim: %w", err)
	}

	return src, nil
}

// tinkerShimTemplate is the generated shim. Its protocol types are a
// hand-maintained duplicate of cmd/zever/tinker_protocol.go's — unavoidable,
// since the shim compiles in the target project's module and cannot import
// this one's internals.
//
// yaegi note: the `+"`zever tinker`"+` client evaluates Go expressions in a
// yaegi REPL (github.com/traefik/yaegi, already in go.mod) and binds
// db/cache/queue/job values that call this shim's verbs. This file only
// emits the shim; the REPL itself lives in tinker.go, out of scope here.
const tinkerShimTemplate = `// Command tinker-shim is the per-project backend for ` + "`zever tinker`" + `.
//
// Code generated by "zever generate tinker". Safe to edit: regenerate with
// --force to discard local changes.
//
// It builds this project's real container and answers a fixed set of verbs
// over a line-delimited JSON protocol: one request per line on stdin, one
// framed response per line on stdout. ` + "`zever tinker`" + ` spawns it with
// ` + "`go run`" + ` and binds db/cache/queue/job values in a yaegi REPL that
// call these verbs.
//
// Run it by hand to poke at the protocol:
//
//	echo '{"verb":"ping"}' | go run ./{{.EntryDir}}
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"time"
	"unsafe"

	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/db"

	app "{{.AppPkg}}"
)

// framePrefix marks a stdout line as a protocol response, so that anything
// else the process writes to stdout (an app logger, for instance) is not
// mistaken for protocol traffic.
//
// MUST STAY IN SYNC WITH cmd/zever/tinker_protocol.go in github.com/zenta-dev/zever.
const framePrefix = ` + "`" + `{{.FramePrefix}}` + "`" + `

// --- wire protocol ----------------------------------------------------
//
// MUST STAY IN SYNC WITH cmd/zever/tinker_protocol.go in github.com/zenta-dev/zever.
// These declarations are duplicated rather than imported because this shim is
// compiled inside this project's module, which cannot reach zever's
// cmd/zever package.

type tinkerRequest struct {
	Verb string          ` + "`json:\"verb\"`" + `
	Args json.RawMessage ` + "`json:\"args,omitempty\"`" + `
}

type tinkerResponse struct {
	Result json.RawMessage ` + "`json:\"result,omitempty\"`" + `
	Error  string          ` + "`json:\"error,omitempty\"`" + `
}

type dbQueryArgs struct {
	SQL  string ` + "`json:\"sql\"`" + `
	Args []any  ` + "`json:\"args,omitempty\"`" + `
}

type dbQueryResult struct {
	Columns []string ` + "`json:\"columns\"`" + `
	Rows    [][]any  ` + "`json:\"rows\"`" + `
}

type dbExecArgs struct {
	SQL  string ` + "`json:\"sql\"`" + `
	Args []any  ` + "`json:\"args,omitempty\"`" + `
}

type dbExecResult struct {
	RowsAffected int64 ` + "`json:\"rows_affected\"`" + `
}

type cacheGetArgs struct {
	Key string ` + "`json:\"key\"`" + `
}

type cacheGetResult struct {
	Value string ` + "`json:\"value\"`" + `
}

type cacheSetArgs struct {
	Key        string ` + "`json:\"key\"`" + `
	Value      string ` + "`json:\"value\"`" + `
	TTLSeconds int    ` + "`json:\"ttl_seconds,omitempty\"`" + `
}

type cacheDeleteArgs struct {
	Key string ` + "`json:\"key\"`" + `
}

type cacheExistsArgs struct {
	Key string ` + "`json:\"key\"`" + `
}

type boolResult struct {
	Value bool ` + "`json:\"value\"`" + `
}

type queuePushArgs struct {
	Topic   string            ` + "`json:\"topic\"`" + `
	Payload string            ` + "`json:\"payload\"`" + `
	Headers map[string]string ` + "`json:\"headers,omitempty\"`" + `
}

type queueLengthArgs struct {
	Topic string ` + "`json:\"topic\"`" + `
}

type int64Result struct {
	Value int64 ` + "`json:\"value\"`" + `
}

type jobDispatchArgs struct {
	Name string          ` + "`json:\"name\"`" + `
	Args json.RawMessage ` + "`json:\"args,omitempty\"`" + `
}

// --- shim -------------------------------------------------------------

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, "tinker-shim:", err)
		os.Exit(1)
	}
}

func run() error {
	c, err := app.New()
	if err != nil {
		return fmt.Errorf("build container: %w", err)
	}

	ctx := context.Background()
	defer func() { _ = c.Close(ctx) }()

	out := bufio.NewWriter(os.Stdout)
	defer func() { _ = out.Flush() }()

	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	for in.Scan() {
		line := in.Bytes()
		if len(line) == 0 {
			continue
		}

		resp := handleLine(ctx, c, line)

		payload, merr := json.Marshal(resp)
		if merr != nil {
			payload = []byte(` + "`" + `{"error":"tinker-shim: marshal response failed"}` + "`" + `)
		}

		if _, werr := out.WriteString(framePrefix); werr != nil {
			return fmt.Errorf("write response: %w", werr)
		}

		if _, werr := out.Write(payload); werr != nil {
			return fmt.Errorf("write response: %w", werr)
		}

		if werr := out.WriteByte('\n'); werr != nil {
			return fmt.Errorf("write response: %w", werr)
		}

		if ferr := out.Flush(); ferr != nil {
			return fmt.Errorf("flush response: %w", ferr)
		}
	}

	if serr := in.Err(); serr != nil {
		return fmt.Errorf("read request: %w", serr)
	}

	return nil
}

// handleLine never returns an error and never panics: a malformed line or an
// unknown verb becomes an error response so the REPL on the other end stays
// usable.
func handleLine(ctx context.Context, c *container.Container, line []byte) tinkerResponse {
	var req tinkerRequest
	if err := json.Unmarshal(line, &req); err != nil {
		return errResponse(fmt.Errorf("malformed request: %w", err))
	}

	result, err := dispatch(ctx, c, req)
	if err != nil {
		return errResponse(err)
	}

	if result == nil {
		return tinkerResponse{}
	}

	payload, err := json.Marshal(result)
	if err != nil {
		return errResponse(fmt.Errorf("marshal result for %q: %w", req.Verb, err))
	}

	return tinkerResponse{Result: payload}
}

func errResponse(err error) tinkerResponse {
	return tinkerResponse{Error: err.Error()}
}

//nolint:cyclop,funlen // a flat verb switch is the clearest shape here
func dispatch(ctx context.Context, c *container.Container, req tinkerRequest) (any, error) {
	switch req.Verb {
	case "ping":
		return "pong", nil

	case "db.query":
		var a dbQueryArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		handle, err := c.DB()
		if err != nil {
			return nil, err
		}

		return queryRows(ctx, handle, a)

	case "db.exec":
		var a dbExecArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		handle, err := c.DB()
		if err != nil {
			return nil, err
		}

		n, err := handle.Exec(ctx, a.SQL, a.Args...)
		if err != nil {
			return nil, err
		}

		return dbExecResult{RowsAffected: n}, nil

	case "cache.get":
		var a cacheGetArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		ch, err := c.Cache()
		if err != nil {
			return nil, err
		}

		v, err := ch.Get(ctx, a.Key)
		if err != nil {
			return nil, err
		}

		return cacheGetResult{Value: string(v)}, nil

	case "cache.set":
		var a cacheSetArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		ch, err := c.Cache()
		if err != nil {
			return nil, err
		}

		var ttl time.Duration
		if a.TTLSeconds > 0 {
			ttl = time.Duration(a.TTLSeconds) * time.Second
		}

		return nil, ch.Set(ctx, a.Key, []byte(a.Value), ttl)

	case "cache.delete":
		var a cacheDeleteArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		ch, err := c.Cache()
		if err != nil {
			return nil, err
		}

		return nil, ch.Delete(ctx, a.Key)

	case "cache.exists":
		var a cacheExistsArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		ch, err := c.Cache()
		if err != nil {
			return nil, err
		}

		ok, err := ch.Exists(ctx, a.Key)
		if err != nil {
			return nil, err
		}

		return boolResult{Value: ok}, nil

	case "queue.push":
		var a queuePushArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		q, err := c.Queue()
		if err != nil {
			return nil, err
		}

		return nil, q.Push(ctx, a.Topic, []byte(a.Payload), a.Headers)

	case "queue.length":
		var a queueLengthArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		q, err := c.Queue()
		if err != nil {
			return nil, err
		}

		n, err := q.Length(ctx, a.Topic)
		if err != nil {
			return nil, err
		}

		return int64Result{Value: n}, nil

	case "job.dispatch":
		var a jobDispatchArgs
		if err := decodeArgs(req, &a); err != nil {
			return nil, err
		}

		d, err := c.Job()
		if err != nil {
			return nil, err
		}

		var jobArgs any
		if len(a.Args) > 0 {
			if err := json.Unmarshal(a.Args, &jobArgs); err != nil {
				return nil, fmt.Errorf("job.dispatch: decode args: %w", err)
			}
		}

		return nil, d.Dispatch(ctx, a.Name, jobArgs)

	default:
		return nil, fmt.Errorf("unknown verb %q", req.Verb)
	}
}

func decodeArgs(req tinkerRequest, dst any) error {
	if len(req.Args) == 0 {
		return nil
	}

	if err := json.Unmarshal(req.Args, dst); err != nil {
		return fmt.Errorf("%s: decode args: %w", req.Verb, err)
	}

	return nil
}

func queryRows(ctx context.Context, handle db.DB, a dbQueryArgs) (dbQueryResult, error) {
	rows, err := handle.Query(ctx, a.SQL, a.Args...)
	if err != nil {
		return dbQueryResult{}, err
	}

	defer func() { _ = rows.Close() }()

	cols := columnNames(rows)
	if len(cols) == 0 {
		return dbQueryResult{}, errors.New("db.query: cannot determine result columns for this db adapter")
	}

	out := dbQueryResult{Columns: cols, Rows: [][]any{}}

	for rows.Next() {
		vals := make([]any, len(cols))

		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}

		if err := rows.Scan(ptrs...); err != nil {
			return dbQueryResult{}, err
		}

		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}

		out.Rows = append(out.Rows, vals)
	}

	if err := rows.Err(); err != nil {
		return dbQueryResult{}, err
	}

	return out, nil
}

// columnNames recovers the result column names from a db.Rows. The db.Rows
// interface now declares Columns(), so the direct assertion is the fast
// path; the reflection probe below covers wrapped *sql.Rows the stock
// adapters may still hide behind.
func columnNames(rows db.Rows) []string {
	type columner interface{ Columns() ([]string, error) }

	if c, ok := rows.(columner); ok {
		if cols, err := c.Columns(); err == nil {
			return cols
		}
	}

	rv := reflect.ValueOf(rows)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return nil
	}

	elem := rv.Elem()
	if elem.Kind() != reflect.Struct {
		return nil
	}

	for _, name := range []string{"rows", "Rows"} {
		fld := elem.FieldByName(name)
		if !fld.IsValid() {
			continue
		}

		inner := fieldValue(fld)
		if inner == nil {
			continue
		}

		if c, ok := inner.(columner); ok {
			if cols, err := c.Columns(); err == nil {
				return cols
			}
		}

		if sr, ok := inner.(*sql.Rows); ok {
			if cols, err := sr.Columns(); err == nil {
				return cols
			}
		}
	}

	return nil
}

func fieldValue(fld reflect.Value) any {
	if fld.CanInterface() {
		return fld.Interface()
	}

	if !fld.CanAddr() {
		return nil
	}

	//nolint:gosec // reading an unexported field of an adapter we own
	return reflect.NewAt(fld.Type(), unsafe.Pointer(fld.UnsafeAddr())).Elem().Interface()
}
`
