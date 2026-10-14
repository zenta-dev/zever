package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// writeTagRepo builds a scratch repo with one conforming module (libs/ok),
// one mistagged module (libs/odd uses an unrelated module path), and the
// two deliberate consumer-proof exceptions (docs/examples with a dashed
// path, examples/external-sms under another org).
func writeTagRepo(t *testing.T) string {
	t.Helper()

	root := t.TempDir()
	write := func(dir, mod string) {
		mustWriteFile(t, filepath.Join(root, dir, "go.mod"),
			"module "+mod+"\n\ngo 1.27.0\n")
		mustWriteFile(t, filepath.Join(root, dir, "doc.go"), "package doc\n")
	}
	write(filepath.Join("libs", "ok"), "github.com/zenta-dev/zever/libs/ok")
	write(filepath.Join("libs", "odd"), "github.com/zenta-dev/zever/unrelated")
	write(filepath.Join("docs", "examples"), "github.com/zenta-dev/zever/docs-examples")
	write(filepath.Join("examples", "external-sms"), "github.com/example/zever-sms")

	return root
}

// TestRunCheckTagPath pins that --check fails on a module whose path does
// not follow the <reldir> tag rule, while the two consumer-proof exceptions
// stay clean.
func TestRunCheckTagPath(t *testing.T) {
	root := writeTagRepo(t)

	code, stdout, _ := runModgraph(t, []string{"-check", "-root", root})
	if code != 1 {
		t.Fatalf("run = %d, want 1 (stdout %q)", code, stdout)
	}
	for _, want := range []string{"libs/odd", "tag path", "github.com/zenta-dev/zever/unrelated"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	for _, leak := range []string{"docs/examples", "external-sms"} {
		if strings.Contains(stdout, leak) {
			t.Errorf("stdout mentions exception %q:\n%s", leak, stdout)
		}
	}
}

// TestRunCheckTagPathClean pins that conforming modules plus the two
// exceptions scan clean.
func TestRunCheckTagPathClean(t *testing.T) {
	root := t.TempDir()
	write := func(dir, mod string) {
		mustWriteFile(t, filepath.Join(root, dir, "go.mod"),
			"module "+mod+"\n\ngo 1.27.0\n")
		mustWriteFile(t, filepath.Join(root, dir, "doc.go"), "package doc\n")
	}
	write(filepath.Join("libs", "ok"), "github.com/zenta-dev/zever/libs/ok")
	write(filepath.Join("docs", "examples"), "github.com/zenta-dev/zever/docs-examples")
	write(filepath.Join("examples", "external-sms"), "github.com/example/zever-sms")

	code, stdout, _ := runModgraph(t, []string{"-check", "-root", root})
	if code != 0 {
		t.Fatalf("run = %d, want 0 (stdout %q)", code, stdout)
	}
	if !strings.Contains(stdout, "module-graph clean") {
		t.Errorf("stdout = %q, want clean message", stdout)
	}
}
