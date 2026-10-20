// Command taggolden regenerates the golden wire-field data of the
// struct-tag guard (tools/tagcheck).
//
// The golden data pins the JSON wire key of every field of every
// registered wire struct. It is derived from immutable git history, never
// from the working tree, so regenerating it can never absorb an
// unintended rename:
//
//   - a struct that existed at the baseline release (v0.7.2, the last
//     release before the v0.8.0 env-only migration stripped the json tags)
//     is pinned at that tag;
//   - a struct created after the baseline release is pinned at the commit
//     that introduced it.
//
// The base revision of every struct is recorded in the generated file, so
// the derivation stays auditable.
//
// Usage:
//
//	taggolden [-root DIR] [-release TAG] [-out FILE] [-check]
//
//	-root DIR     repository root (default ".")
//	-release TAG  baseline release tag (default "v0.7.2")
//	-out FILE     output file (default "<root>/tools/tagcheck/golden_wire_fields.txt")
//	-check        verify the committed file matches the generated data
//	              instead of writing it (exit 1 on drift)
//
// The command needs the full git history and the baseline tag:
//
//	git fetch --tags --unshallow
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zenta-dev/zever/tools/tagcheck/internal/wire"
)

// header documents the generated file. It must stay byte-stable: the
// generator is expected to reproduce the committed file exactly.
const header = `# golden_wire_fields.txt — pinned JSON wire keys for the registered
# wire structs of the struct-tag guard (tools/tagcheck).
#
# Format: <repo-relative package dir><TAB><struct name><TAB><Go field name><TAB><wire key><TAB><base revision>
# Blank lines and # comments are ignored.
#
# The wire key is the JSON object key encoding/json writes for the field:
# the name part of its json tag. A key of "-" means the field is kept off
# the wire (json:"-"). The base revision is the immutable git revision the
# key was taken from: the baseline release for structs that existed at
# that release, the commit that introduced the struct otherwise.
#
# tagcheck fails when a pinned field exposes a different key, an empty key
# (the wire falls back to the Go field name), or is dropped from the wire.
#
# How this file was derived (reproducible):
#   go -C tools/tagcheck run ./cmd/taggolden -root .
# The generator reads the registry (golden_wire_structs.txt) and the git
# history; it never reads the working tree for the pinned keys.
`

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Stdout, os.Stderr))
}

// run regenerates the golden field file and returns the process exit code:
// 0 when the file was written or already matches, 1 on drift, 2 on error.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("taggolden", flag.ContinueOnError)
	flags.SetOutput(stderr)

	root := flags.String("root", ".", "repository root")
	release := flags.String("release", "v0.7.2", "baseline release tag")
	out := flags.String("out", "", "output file")
	check := flags.Bool("check", false, "verify the committed file instead of writing it")
	list := flags.Bool("list-unregistered", false, "list the wire structs of the tree that are not registered")

	if err := flags.Parse(args); err != nil {
		return 2
	}

	registryPath := filepath.Join(*root, "tools", "tagcheck", "golden_wire_structs.txt")
	outPath := *out
	if outPath == "" {
		outPath = filepath.Join(*root, "tools", "tagcheck", "golden_wire_fields.txt")
	}

	entries, err := loadRegistry(registryPath)
	if err != nil {
		fmt.Fprintf(stderr, "taggolden: %v\n", err)
		return 2
	}

	if *list {
		return listUnregistered(*root, entries, stdout, stderr)
	}

	git := &repo{dir: *root}

	data, err := generate(ctx, git, entries, *release)
	if err != nil {
		fmt.Fprintf(stderr, "taggolden: %v\n", err)
		return 2
	}

	if *check {
		committed, err := os.ReadFile(outPath)
		if err != nil {
			fmt.Fprintf(stderr, "taggolden: read %s: %v\n", outPath, err)
			return 2
		}

		if !bytes.Equal(committed, data) {
			fmt.Fprintf(stderr, "taggolden: %s is out of date; regenerate it with: go -C tools/tagcheck run ./cmd/taggolden -root .\n", outPath)
			return 1
		}

		fmt.Fprintf(stdout, "taggolden: %s is up to date (%d pinned keys)\n", outPath, countKeys(data))

		return 0
	}

	if err := os.WriteFile(outPath, data, 0o600); err != nil {
		fmt.Fprintf(stderr, "taggolden: write %s: %v\n", outPath, err)
		return 2
	}

	fmt.Fprintf(stdout, "taggolden: wrote %s (%d pinned keys)\n", outPath, countKeys(data))

	return 0
}

// listUnregistered prints the wire structs of the tree that are not
// registered in the registry. It is the helper for the registry: a struct
// that exposes json keys belongs in golden_wire_structs.txt, so its keys
// get pinned, unless it is excepted with a justification.
func listUnregistered(root string, entries []entry, stdout, stderr io.Writer) int {
	registered := map[string]bool{}
	for _, e := range entries {
		registered[e.dir+"\t"+e.name] = true
	}

	excepted, err := loadExcepted(filepath.Join(root, "tools", "tagcheck", "golden_wire_exceptions.txt"))
	if err != nil {
		fmt.Fprintf(stderr, "taggolden: %v\n", err)
		return 2
	}

	var missing []string

	err = wire.WalkStructs(root, func(dir, name string, fields []wire.Field) error {
		if registered[dir+"\t"+name] || excepted[dir+"\t"+name] {
			return nil
		}

		for _, field := range fields {
			if field.State() == wire.StateNamed {
				missing = append(missing, dir+"\t"+name)

				return nil
			}
		}

		return nil
	})
	if err != nil {
		fmt.Fprintf(stderr, "taggolden: %v\n", err)
		return 2
	}

	sort.Strings(missing)

	for _, item := range missing {
		fmt.Fprintln(stdout, item)
	}

	if len(missing) == 0 {
		fmt.Fprintln(stdout, "every wire struct of the tree is registered")
	}

	return 0
}

// loadExcepted parses the exception list of the registry: tab-separated
// package dir, struct name, and the justification.
func loadExcepted(path string) (map[string]bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}

		return nil, fmt.Errorf("read exception list: %w", err)
	}

	excepted := map[string]bool{}

	for _, line := range strings.Split(string(content), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		parts := strings.Split(text, "\t")
		if len(parts) < 2 {
			return nil, fmt.Errorf("taggolden: malformed exception line %q", text)
		}

		excepted[parts[0]+"\t"+parts[1]] = true
	}

	return excepted, nil
}

// countKeys counts the data lines of the generated file.
func countKeys(data []byte) int {
	count := 0

	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		count++
	}

	return count
}

// pinnedKey returns the wire key pinned for a field: the JSON object key
// encoding/json writes for it, or "-" when the field is kept off the wire.
func pinnedKey(field wire.Field) string {
	key, onWire := field.Key()
	if !onWire {
		return "-"
	}

	return key
}

// entry is one registered wire struct.
type entry struct {
	dir  string
	name string
}

// loadRegistry parses the wire-struct registry: tab-separated package dir,
// struct name, and a documentation column.
func loadRegistry(path string) ([]entry, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read registry: %w", err)
	}

	var entries []entry

	seen := map[string]bool{}

	for _, line := range strings.Split(string(content), "\n") {
		text := strings.TrimSpace(line)
		if text == "" || strings.HasPrefix(text, "#") {
			continue
		}

		parts := strings.Split(text, "\t")
		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			return nil, fmt.Errorf("taggolden: malformed registry line %q", text)
		}

		key := parts[0] + "\t" + parts[1]
		if seen[key] {
			continue
		}

		seen[key] = true

		entries = append(entries, entry{dir: parts[0], name: parts[1]})
	}

	return entries, nil
}

// generate builds the golden field file content.
func generate(ctx context.Context, git *repo, entries []entry, release string) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(header)

	for _, e := range entries {
		fields, base, err := git.resolve(ctx, e.dir, e.name, release)
		if err != nil {
			return nil, err
		}

		for _, field := range fields {
			fmt.Fprintf(&buf, "%s\t%s\t%s\t%s\t%s\n", e.dir, e.name, field.Name, pinnedKey(field), base)
		}
	}

	return buf.Bytes(), nil
}

// repo reads files and history from a git repository.
type repo struct {
	dir string
}

// run executes a git command in the repository and returns its output.
func (r *repo) run(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir

	var stdout, stderr bytes.Buffer

	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// resolve returns the pinned fields of a struct and the base revision they
// were taken from: the baseline release when the struct existed there, the
// commit that introduced it otherwise.
func (r *repo) resolve(ctx context.Context, dir, name, release string) ([]wire.Field, string, error) {
	fields, ok, err := r.fieldsAt(ctx, release, dir, name)
	if err != nil {
		return nil, "", err
	}

	if ok {
		return fields, release, nil
	}

	commits, err := r.candidates(ctx, dir, name)
	if err != nil {
		return nil, "", err
	}

	for _, commit := range commits {
		fields, ok, err := r.fieldsAt(ctx, commit, dir, name)
		if err != nil {
			return nil, "", err
		}

		if !ok {
			continue
		}

		short, err := r.short(ctx, commit)
		if err != nil {
			return nil, "", err
		}

		return fields, short, nil
	}

	return nil, "", fmt.Errorf("taggolden: wire struct %s in %s carries no json tags in the release %s nor in any commit of its history", name, dir, release)
}

// candidates returns the commits that touched the declaration of a struct,
// oldest first.
func (r *repo) candidates(ctx context.Context, dir, name string) ([]string, error) {
	out, err := r.run(ctx, "log", "--reverse", "--format=%H", "-G", "^type "+name, "--", dir)
	if err != nil {
		return nil, err
	}

	var commits []string

	for _, line := range strings.Split(out, "\n") {
		commit := strings.TrimSpace(line)
		if commit == "" {
			continue
		}

		commits = append(commits, commit)
	}

	return commits, nil
}

// short returns the abbreviated commit id of a revision.
func (r *repo) short(ctx context.Context, rev string) (string, error) {
	out, err := r.run(ctx, "rev-parse", "--short=12", rev)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(out), nil
}

// fieldsAt returns the exported fields of a struct at a revision. It
// reports false when the struct does not exist there or when one of its
// exported fields carries no json tag: only fully tagged structs are wire
// contracts and can be pinned.
func (r *repo) fieldsAt(ctx context.Context, rev, dir, name string) ([]wire.Field, bool, error) {
	files, err := r.filesAt(ctx, rev, dir)
	if err != nil {
		return nil, false, err
	}

	for _, file := range files {
		content, err := r.run(ctx, "show", rev+":"+file)
		if err != nil {
			return nil, false, err
		}

		fset := token.NewFileSet()

		f, err := parser.ParseFile(fset, file, content, parser.SkipObjectResolution)
		if err != nil {
			continue
		}

		fields, ok := wire.Fields(f, name)
		if !ok {
			continue
		}

		for _, field := range fields {
			if field.State() == wire.StateMissing {
				return nil, false, nil
			}
		}

		return fields, true, nil
	}

	return nil, false, nil
}

// filesAt lists the Go files of a package directory at a revision.
func (r *repo) filesAt(ctx context.Context, rev, dir string) ([]string, error) {
	out, err := r.run(ctx, "ls-tree", "-r", "--name-only", rev, "--", dir)
	if err != nil {
		return nil, err
	}

	var files []string

	for _, line := range strings.Split(out, "\n") {
		file := strings.TrimSpace(line)
		if file == "" || !strings.HasSuffix(file, ".go") {
			continue
		}

		if filepath.ToSlash(filepath.Dir(file)) != dir {
			continue
		}

		files = append(files, file)
	}

	sort.Strings(files)

	return files, nil
}
