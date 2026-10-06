package main

import (
	"bytes"
	"go/ast"
	"go/token"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestRunViolations pins the in-process CLI contract: violation lines on
// stdout, summary on stderr, exit 1.
func TestRunViolations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, root, "pkg/violations.go", mustFixture(t, "violations.txt"))

	var stdout, stderr bytes.Buffer
	if code := run([]string{root}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, want 1 (stdout %q, stderr %q)", code, stdout.String(), stderr.String())
	}

	for _, want := range []string{
		"pkg/violations.go:9: unprefixed:",
		"pkg/violations.go:11: bracket-prefix:",
		"pkg/violations.go:12: double-%w:",
		"pkg/violations.go:13: panic:",
	} {
		if !strings.Contains(stdout.String(), want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout.String())
		}
	}

	if !strings.Contains(stderr.String(), "violation(s)") {
		t.Errorf("stderr = %q, want violation summary", stderr.String())
	}
}

// TestRunClean pins that a clean tree exits 0 with silent outputs.
func TestRunClean(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, root, "pkg/clean.go", mustFixture(t, "clean.txt"))

	var stdout, stderr bytes.Buffer
	if code := run([]string{root}, &stdout, &stderr); code != 0 {
		t.Fatalf("run = %d, want 0 (stdout %q, stderr %q)", code, stdout.String(), stderr.String())
	}

	if stdout.String() != "" {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}

	if stderr.String() != "" {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

// TestRunScanError pins that a missing root exits 2 with an errscan-prefixed
// message on stderr and silent stdout.
func TestRunScanError(t *testing.T) {
	t.Parallel()

	var stdout, stderr bytes.Buffer
	code := run([]string{filepath.Join(t.TempDir(), "does-not-exist")}, &stdout, &stderr)

	if code != 2 {
		t.Fatalf("run = %d, want 2", code)
	}

	if stdout.String() != "" {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}

	if !strings.Contains(stderr.String(), "errscan:") {
		t.Errorf("stderr = %q, want errscan prefix", stderr.String())
	}
}

// TestRunDefaultRoot pins that no args scans the working directory. It must
// not run in parallel: chdir is process-global.
func TestRunDefaultRoot(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, root, "pkg/clean.go", mustFixture(t, "clean.txt"))

	t.Chdir(root)

	var stdout, stderr bytes.Buffer
	if code := run(nil, &stdout, &stderr); code != 0 {
		t.Fatalf("run(nil) = %d, want 0 (stdout %q, stderr %q)", code, stdout.String(), stderr.String())
	}
}

// TestRunSortsViolations pins the path, line, then rule ordering applied
// before printing.
func TestRunSortsViolations(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	mustWrite(t, root, "b/b.go", "package b\n\nimport \"errors\"\n\nvar _ = errors.New(\"x\")\n")
	mustWrite(t, root, "a/a.go", "package a\n\nimport \"errors\"\n\nvar _ = errors.New(\"y\")\n")

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if code := run([]string{root}, &stdout, &stderr); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}

	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("stdout lines = %q, want 2", stdout.String())
	}

	if !strings.HasPrefix(lines[0], "a/a.go:") || !strings.HasPrefix(lines[1], "b/b.go:") {
		t.Errorf("unsorted lines = %q", stdout.String())
	}
}

// TestCheckFileReadError pins that an unreadable path is a hard error.
func TestCheckFileReadError(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "does-not-exist.go")
	if _, err := checkFile("pkg/missing.go", missing); err == nil {
		t.Fatal("checkFile(missing) = nil error, want read error")
	}
}

// TestCheckFileMixedAllowGroup pins that a non-marker comment sharing a group
// with an allow comment does not block suppression.
func TestCheckFileMixedAllowGroup(t *testing.T) {
	t.Parallel()

	src := "package p\n\n// plain note\n// errscan:allow\n\nimport \"errors\"\n\nfunc f() {\n\t_ = errors.New(\"x\")\n}\n"
	if vs := checkSource(t, "pkg/s.go", src); len(vs) != 0 {
		t.Errorf("mixed allow group = %#v, want none", vs)
	}
}

// TestCheckCallUnquotableLiteral pins the defensive Unquote branch: a string
// literal the parser would never produce is skipped without a violation.
func TestCheckCallUnquotableLiteral(t *testing.T) {
	t.Parallel()

	call := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   ast.NewIdent("errors"),
			Sel: ast.NewIdent("New"),
		},
		Args: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: "\"\\x\""},
		},
	}

	var added []string
	add := func(rule string, _ int, _ string, _ ...any) {
		added = append(added, rule)
	}

	checkCall(call, false, false, false, add, 1)

	if len(added) != 0 {
		t.Errorf("checkCall(unquotable) added %v, want none", added)
	}
}

// TestCheckCallErrorfPrefixRules pins that prefixed messages skip unprefixed
// while double-%w still fires.
func TestCheckCallErrorfPrefixRules(t *testing.T) {
	t.Parallel()

	src := "package p\n\nimport \"fmt\"\n\nvar _ = fmt.Errorf(\"p: %w %w\", nil, nil)\n"
	got := vkeys(checkSource(t, "pkg/s.go", src))
	want := []vkey{{path: "pkg/s.go", line: 5, rule: "double-%w"}}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("errorf rules = %#v, want %#v", got, want)
	}
}
