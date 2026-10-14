package ffmpeg

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/media"
)

// TestValidatePath pins the P2-11g input allowlist: known media extensions
// pass, while flag-looking and unknown paths fail closed.
func TestValidatePath(t *testing.T) {
	t.Parallel()

	for _, p := range []string{"in.mp3", "in.mov", "out.mp4", "v.mp4", "t.jpg", "a.wav", "clip.webm"} {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil", p, err)
		}
	}

	cases := []struct {
		name string
		path string
	}{
		{"empty", ""},
		{"blank", "   "},
		{"leading dash input", "-evil.mp3"},
		{"leading dash output", "-filter_complex.mp4"},
		{"unknown extension", "in.zzz"},
		{"no extension", "infile"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if err := ValidatePath(tc.path); !errors.Is(err, ErrInvalidPath) {
				t.Errorf("ValidatePath(%q) = %v, want ErrInvalidPath", tc.path, err)
			}
		})
	}
}

// TestBuildArgs_SeparatorBeforeOutput pins the P2-11f fix: the output path
// is always preceded by `--` so a leading-dash name cannot parse as a flag.
func TestBuildArgs_SeparatorBeforeOutput(t *testing.T) {
	t.Parallel()

	av := BuildArgs("in.wav", "out.mp3", Spec{Kind: media.KindAudio, Format: "mp3"})
	if len(av) < 2 || av[len(av)-2] != "--" || av[len(av)-1] != "out.mp3" {
		t.Errorf("BuildArgs AV tail = %q, want [... -- out.mp3]", av)
	}

	thumb := BuildArgs("v.mp4", "t.jpg", Spec{Kind: media.KindVideo, Format: "jpg"})
	if len(thumb) < 2 || thumb[len(thumb)-2] != "--" || thumb[len(thumb)-1] != "t.jpg" {
		t.Errorf("BuildArgs thumbnail tail = %q, want [... -- t.jpg]", thumb)
	}
}

// TestProbe_RejectsUnsafePath proves Probe validates before exec: even with
// a missing binary, an unsafe path reports ErrInvalidPath rather than
// reaching the tool lookup.
func TestProbe_RejectsUnsafePath(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "no-such-ffprobe")
	for _, path := range []string{"-evil.mp3", "in.zzz"} {
		if _, err := Probe(t.Context(), missing, path); !errors.Is(err, ErrInvalidPath) {
			t.Errorf("Probe(%q) err = %v, want ErrInvalidPath", path, err)
		}
	}
}

// TestProbe_SeparatorPassedToBinary proves Probe inserts `--` before the
// path in the ffprobe argv.
func TestProbe_SeparatorPassedToBinary(t *testing.T) {
	t.Parallel()

	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeScript(t, "#!/bin/sh\necho \"$@\" > "+strconv.Quote(argsFile)+"\ncat <<'EOF'\n{\"format\":{\"format_name\":\"mp3\",\"duration\":\"7.5\"},\"streams\":[]}\nEOF\n")

	if _, err := Probe(t.Context(), bin, "in.mp3"); err != nil {
		t.Fatalf("Probe failed: %v", err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	fields := strings.Fields(string(raw))
	idx := slices.Index(fields, "--")
	if idx < 0 || idx+1 >= len(fields) || fields[idx+1] != "in.mp3" {
		t.Errorf("ffprobe argv = %q, want -- before in.mp3", fields)
	}
}

// TestRunTranscode_RejectsUnsafeArgs proves RunTranscode fails closed on
// flag-looking inputs/outputs without spawning the binary.
func TestRunTranscode_RejectsUnsafeArgs(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "no-such-ffmpeg")
	cases := []struct {
		name string
		argv []string
	}{
		{"empty argv", nil},
		{"dash output", []string{"-y", "-i", "in.wav", "-evil.mp3"}},
		{"dash input", []string{"-y", "-i", "-evil.wav", "out.mp3"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := RunTranscode(t.Context(), missing, tc.argv)
			if tc.name == "empty argv" {
				if !errors.Is(err, ErrTranscodeFailed) {
					t.Errorf("RunTranscode(nil) err = %v, want ErrTranscodeFailed", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalidPath) {
				t.Errorf("RunTranscode(%q) err = %v, want ErrInvalidPath", tc.argv, err)
			}
		})
	}
}

// TestRunTranscode_InsertsSeparator proves RunTranscode adds `--` before the
// output for hand-built argv vectors that predate BuildArgs separators.
func TestRunTranscode_InsertsSeparator(t *testing.T) {
	t.Parallel()

	argsFile := filepath.Join(t.TempDir(), "args")
	bin := writeScript(t, "#!/bin/sh\necho \"$@\" > "+strconv.Quote(argsFile)+"\nexit 0\n")

	argv := []string{"-y", "-i", "in.wav", "-c:a", "libmp3lame", "out.mp3"}
	if err := RunTranscode(t.Context(), bin, argv); err != nil {
		t.Fatalf("RunTranscode failed: %v", err)
	}

	raw, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args: %v", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) < 2 || fields[len(fields)-2] != "--" || fields[len(fields)-1] != "out.mp3" {
		t.Errorf("ffmpeg argv tail = %q, want [... -- out.mp3]", fields)
	}
}

// TestCheckOutputSize pins the P2-11h output cap helper: missing files are
// skipped (stub binaries create none), oversized files fail.
func TestCheckOutputSize(t *testing.T) {
	t.Parallel()

	if err := checkOutputSize([]string{"-y", "no-such-output.mp3"}, 10); err != nil {
		t.Errorf("checkOutputSize(missing) = %v, want nil", err)
	}

	big := filepath.Join(t.TempDir(), "big.mp3")
	if err := os.WriteFile(big, make([]byte, 64), 0o600); err != nil {
		t.Fatalf("write big: %v", err)
	}
	if err := checkOutputSize([]string{"-y", big}, 63); !errors.Is(err, ErrOutputTooLarge) {
		t.Errorf("checkOutputSize(64B, limit 63) = %v, want ErrOutputTooLarge", err)
	}
	if err := checkOutputSize([]string{"-y", big}, 64); err != nil {
		t.Errorf("checkOutputSize(64B, limit 64) = %v, want nil", err)
	}
}

// TestWithDefaultTimeout pins the P2-11h timeout default: a bare context
// gains a deadline, while a caller deadline is left untouched.
func TestWithDefaultTimeout(t *testing.T) {
	t.Parallel()

	ctx, cancel := withDefaultTimeout(t.Context(), 30*time.Second)
	defer cancel()
	if _, ok := ctx.Deadline(); !ok {
		t.Error("withDefaultTimeout(bare) has no deadline, want one")
	}

	base, baseCancel := context.WithTimeout(t.Context(), time.Hour)
	defer baseCancel()
	got, cancel2 := withDefaultTimeout(base, time.Second)
	defer cancel2()
	dl, ok := got.Deadline()
	if !ok {
		t.Fatal("withDefaultTimeout(deadline) lost the deadline")
	}
	if baseDl, _ := base.Deadline(); !dl.Equal(baseDl) {
		t.Errorf("deadline = %v, want caller's %v", dl, baseDl)
	}
}
