package latex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/zenta-dev/zever/core/document"
)

const (
	bwrapName    = "bwrap"
	probeTimeout = 5 * time.Second
)

// systemBinds are read-only system trees TeX and poppler need; missing ones
// are skipped (bwrap --ro-bind-try). Notably /etc itself is NOT bound, so
// files like /etc/passwd and /etc/hostname do not exist inside the sandbox.
var systemBinds = []string{
	"/usr", "/bin", "/sbin", "/lib", "/lib32", "/lib64", "/opt",
	"/etc/texmf", "/etc/texlive", "/etc/fonts", "/etc/ld.so.cache", "/etc/ld.so.conf", "/etc/ld.so.conf.d",
	"/var/lib/texmf", "/var/cache/fontconfig", "/usr/share/texmf",
}

// deps are the injectable environment probes so tests can fake bubblewrap.
type deps struct {
	// lookPath resolves an executable (exec.LookPath).
	lookPath func(string) (string, error)
	// probe verifies the resolved bwrap can actually create a sandbox
	// (unprivileged user namespaces may be disabled).
	probe func(ctx context.Context, bwrap string) error
	// warn reports a degraded-security condition once per driver.
	warn func(msg string)
}

func defaultDeps() deps {
	return deps{
		lookPath: exec.LookPath,
		probe:    probeBwrap,
		warn:     func(msg string) { slog.Warn(msg) },
	}
}

func probeBwrap(ctx context.Context, bwrap string) error {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	// binary resolved via LookPath; fixed argv; no shell.
	out, err := exec.CommandContext(ctx, bwrap, "--unshare-all", "--die-with-parent",
		"--ro-bind", "/", "/", "--", "true").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// sandbox runs commands inside bubblewrap with no network, read-only system
// trees and a single writable directory (the per-render temp dir).
type sandbox struct {
	bwrap string
	binds []string
}

// newSandbox applies the configured mode. It returns nil (no sandbox) for
// "off", or for "auto" when bwrap is missing or unusable (after warning once).
func newSandbox(mode, tex string, d deps) (*sandbox, error) {
	if mode == document.LatexSandboxOff {
		return nil, nil //nolint:nilnil // nil sandbox means unsandboxed by design.
	}
	bwrap, err := d.lookPath(bwrapName)
	if err == nil && d.probe != nil {
		err = d.probe(context.Background(), bwrap)
	}
	if err != nil {
		if mode == document.LatexSandboxRequired {
			return nil, errors.Join(ErrSandboxUnavailable, err)
		}
		d.warn("latex: bubblewrap unavailable; running TeX without an OS sandbox, relying on the source pre-scan only: " + err.Error())
		return nil, nil //nolint:nilnil // nil sandbox means unsandboxed by design.
	}
	sb := &sandbox{bwrap: bwrap, binds: append([]string(nil), systemBinds...)}
	sb.binds = append(sb.binds, toolBinds(tex)...)
	for _, kv := range os.Environ() {
		if v, ok := strings.CutPrefix(kv, "TEXMF"); ok {
			if _, val, ok := strings.Cut(v, "="); ok {
				for _, p := range strings.FieldsFunc(val, func(r rune) bool { return r == os.PathListSeparator }) {
					if filepath.IsAbs(p) {
						sb.binds = append(sb.binds, p)
					}
				}
			}
		}
	}
	return sb, nil
}

// toolBinds returns the directories to expose for an executable: for a
// TeX Live layout (.../texlive/<year>/bin/<arch>/tool) the whole year tree,
// otherwise the install prefix two levels above the resolved binary.
func toolBinds(exe string) []string {
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		resolved = exe
	}
	parts := strings.Split(filepath.ToSlash(resolved), "/")
	for i, p := range parts {
		if p == "texlive" && i+1 < len(parts) {
			return []string{"/" + filepath.ToSlash(filepath.Join(parts[:i+2]...))}
		}
	}
	if prefix := filepath.Dir(filepath.Dir(resolved)); prefix != "/" && prefix != "." {
		return []string{prefix, filepath.Dir(resolved)}
	}
	return []string{filepath.Dir(resolved)}
}

// wrap rewrites name+argv to run inside the sandbox with dir writable and
// as the working directory.
func (s *sandbox) wrap(dir, name string, argv []string) (string, []string) {
	out := []string{"--unshare-all", "--die-with-parent", "--new-session", "--tmpfs", "/tmp"}
	for _, b := range append(append([]string(nil), s.binds...), toolBinds(name)...) {
		out = append(out, "--ro-bind-try", b, b)
	}
	out = append(out,
		"--proc", "/proc", "--dev", "/dev",
		"--bind", dir, dir, "--chdir", dir,
		"--", name)
	return s.bwrap, append(out, argv...)
}
