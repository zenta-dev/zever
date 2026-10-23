package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/shared/codec"
)

// TestRunBuildOverlayWiring pins the full non-compiler path: resolve →
// filter → overlay JSON → build args → size report. The compiler itself is
// faked (a real `go build` of cmd/zever is minutes, not a unit test).
func TestRunBuildOverlayWiring(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	dir := t.TempDir()

	orig := filepath.Join(dir, "adapters.go")
	if err := os.WriteFile(orig, []byte(stubAdaptersSource), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer

	var gotDir string

	var gotArgs []string

	var overlayRaw []byte

	var slimRaw []byte

	fakeBin := filepath.Join(dir, "fake-slim")

	err := runBuildWith(ctx, buildOptions{
		adapters:  "cache",
		out:       fakeBin,
		stdout:    &out,
		sourceDir: dir,
		build: func(_ context.Context, dir string, args []string) ([]byte, error) {
			gotDir = dir
			gotArgs = args

			for i, a := range args {
				if a == "-overlay" && i+1 < len(args) {
					raw, err := os.ReadFile(args[i+1])
					if err != nil {
						t.Error(err)
					}

					overlayRaw = raw

					var overlay struct {
						Replace map[string]string `json:"Replace"`
					}

					if err := codec.Unmarshal(raw, &overlay); err != nil {
						t.Error(err)
					} else {
						for _, slimPath := range overlay.Replace {
							slim, err := os.ReadFile(slimPath)
							if err != nil {
								t.Error(err)
							}

							slimRaw = slim
						}
					}
				}
			}

			if err := os.WriteFile(fakeBin, []byte("binary"), 0o600); err != nil {
				t.Error(err)
			}

			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("runBuildWith error = %v", err)
	}

	if gotDir != dir {
		t.Errorf("build dir = %q, want %q", gotDir, dir)
	}

	if overlayRaw == nil {
		t.Fatalf("build args missing -overlay: %v", gotArgs)
	}

	raw := overlayRaw

	var overlay struct {
		Replace map[string]string `json:"Replace"`
	}

	if err := codec.Unmarshal(raw, &overlay); err != nil {
		t.Fatalf("overlay JSON invalid: %v\n%s", err, raw)
	}

	if _, ok := overlay.Replace[orig]; !ok {
		t.Fatalf("overlay missing %q: %s", orig, raw)
	}

	if slimRaw == nil {
		t.Fatalf("overlay has no slim replacement: %s", raw)
	}

	if strings.Contains(string(slimRaw), "adapters/queue") {
		t.Errorf("slim source keeps queue:\n%s", slimRaw)
	}

	if !strings.Contains(string(slimRaw), "adapters/cache/memory") {
		t.Errorf("slim source drops cache:\n%s", slimRaw)
	}

	if !strings.Contains(out.String(), "6 bytes") {
		t.Errorf("report missing size:\n%s", out.String())
	}
}

// TestRunBuildFullSetSkipsOverlay pins that an empty --adapters builds the
// full CLI with a plain `go build` (no overlay flag).
func TestRunBuildFullSetSkipsOverlay(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	dir := t.TempDir()

	orig := filepath.Join(dir, "adapters.go")
	if err := os.WriteFile(orig, []byte(stubAdaptersSource), 0o600); err != nil {
		t.Fatal(err)
	}

	fakeBin := filepath.Join(dir, "fake-full")

	var gotArgs []string

	err := runBuildWith(ctx, buildOptions{
		out:       fakeBin,
		stdout:    &bytes.Buffer{},
		sourceDir: dir,
		build: func(_ context.Context, _ string, args []string) ([]byte, error) {
			gotArgs = args

			if err := os.WriteFile(fakeBin, []byte("binary"), 0o600); err != nil {
				t.Error(err)
			}

			return nil, nil
		},
	})
	if err != nil {
		t.Fatalf("runBuildWith error = %v", err)
	}

	for _, a := range gotArgs {
		if a == "-overlay" {
			t.Fatalf("full build passes -overlay: %v", gotArgs)
		}
	}
}
