package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustWriteUninstallFile(t *testing.T, path string) {
	t.Helper()

	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestParseUninstallAnswer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"y", "y", true},
		{"yes", "yes", true},
		{"upper Y", "Y", true},
		{"upper YES", "YES", true},
		{"padded", "  yes  \n", true},
		{"empty defaults no", "", false},
		{"newline defaults no", "\n", false},
		{"n", "n", false},
		{"no", "no", false},
		{"yep", "yep", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := parseUninstallAnswer(tc.input); got != tc.want {
				t.Errorf("parseUninstallAnswer(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestPlanUninstall(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		keepLSP  bool
		goos     string
		lsp      []string
		wantRm   []string
		wantMan  int
		wantSelf bool
	}{
		{name: "posix removes self and lsp", keepLSP: false, goos: "linux", lsp: []string{"/bin/zever-lsp"}, wantRm: []string{"/bin/zever", "/bin/zever-lsp"}, wantSelf: true},
		{name: "posix keep-lsp drops lsp", keepLSP: true, goos: "linux", lsp: []string{"/bin/zever-lsp"}, wantRm: []string{"/bin/zever"}, wantSelf: true},
		{name: "windows self is manual", keepLSP: false, goos: "windows", lsp: []string{`C:\bin\zever-lsp.exe`}, wantRm: []string{`C:\bin\zever-lsp.exe`}, wantMan: 1},
		{name: "windows keep-lsp nothing automatic", keepLSP: true, goos: "windows", lsp: []string{`C:\bin\zever-lsp.exe`}, wantMan: 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exe := "/bin/zever"
			if tc.goos == "windows" {
				exe = `C:\bin\zever.exe`
			}

			remove, manual := planUninstall(exe, tc.lsp, tc.keepLSP, tc.goos)

			if len(remove) != len(tc.wantRm) {
				t.Fatalf("remove = %v, want %v", remove, tc.wantRm)
			}

			for i := range tc.wantRm {
				if remove[i] != tc.wantRm[i] {
					t.Fatalf("remove = %v, want %v", remove, tc.wantRm)
				}
			}

			if len(manual) != tc.wantMan {
				t.Fatalf("manual = %v, want %d step(s)", manual, tc.wantMan)
			}

			for _, step := range manual {
				if !strings.Contains(step, "del ") {
					t.Errorf("manual step %q missing del", step)
				}
			}
		})
	}
}

func TestFindUninstallLSPFakePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	mustWriteUninstallFile(t, filepath.Join(dir, "zever-lsp"))

	found := findUninstallLSP(dir, "linux")
	if len(found) != 1 || found[0] != filepath.Join(dir, "zever-lsp") {
		t.Fatalf("found = %v, want the fake zever-lsp", found)
	}

	if got := findUninstallLSP(t.TempDir(), "linux"); len(got) != 0 {
		t.Fatalf("empty PATH dir found = %v, want none", got)
	}

	winDir := t.TempDir()
	mustWriteUninstallFile(t, filepath.Join(winDir, "zever-lsp.exe"))

	if got := findUninstallLSP(winDir, "linux"); len(got) != 0 {
		t.Fatalf("linux lookup found windows binary: %v", got)
	}

	if got := findUninstallLSP(winDir, "windows"); len(got) != 1 {
		t.Fatalf("windows lookup found = %v, want one", got)
	}
}

func uninstallFixture(t *testing.T, withLSP bool) (exe string, pathEnv string) {
	t.Helper()

	binDir := t.TempDir()
	exe = filepath.Join(binDir, "zever")
	mustWriteUninstallFile(t, exe)

	lspDir := t.TempDir()
	if withLSP {
		mustWriteUninstallFile(t, filepath.Join(lspDir, "zever-lsp"))
	}

	return exe, binDir + string(os.PathListSeparator) + lspDir
}

func TestRunUninstallDryRun(t *testing.T) {
	t.Parallel()

	exe, pathEnv := uninstallFixture(t, true)

	var out bytes.Buffer

	err := runUninstallWith(uninstallOptions{
		dryRun:     true,
		out:        &out,
		isTerminal: func() bool { return false },
		executable: func() (string, error) { return exe, nil },
		pathEnv:    pathEnv,
		hasPathEnv: true,
		goos:       "linux",
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "would remove "+exe) {
		t.Errorf("dry-run output missing self plan:\n%s", got)
	}

	if !strings.Contains(got, "would remove ") || !strings.Contains(got, "zever-lsp") {
		t.Errorf("dry-run output missing lsp plan:\n%s", got)
	}

	if _, err := os.Stat(exe); err != nil {
		t.Errorf("dry-run deleted the binary: %v", err)
	}
}

func TestRunUninstallNeedsConfirmNonTTY(t *testing.T) {
	t.Parallel()

	exe, pathEnv := uninstallFixture(t, true)

	var out bytes.Buffer

	err := runUninstallWith(uninstallOptions{
		out:        &out,
		isTerminal: func() bool { return false },
		executable: func() (string, error) { return exe, nil },
		pathEnv:    pathEnv,
		hasPathEnv: true,
		goos:       "linux",
	})
	if !errors.Is(err, ErrUninstallNeedsConfirm) {
		t.Fatalf("err = %v, want ErrUninstallNeedsConfirm", err)
	}

	if !strings.Contains(err.Error(), "zever uninstall --yes") {
		t.Errorf("err %q missing the exact re-run command", err.Error())
	}

	if _, serr := os.Stat(exe); serr != nil {
		t.Errorf("refused run deleted the binary: %v", serr)
	}
}

func TestRunUninstallYesRemovesFiles(t *testing.T) {
	t.Parallel()

	exe, pathEnv := uninstallFixture(t, true)
	lsp := filepath.Join(strings.Split(pathEnv, string(os.PathListSeparator))[1], "zever-lsp")

	var out bytes.Buffer

	err := runUninstallWith(uninstallOptions{
		yes:        true,
		out:        &out,
		isTerminal: func() bool { return false },
		executable: func() (string, error) { return exe, nil },
		pathEnv:    pathEnv,
		hasPathEnv: true,
		goos:       "linux",
	})
	if err != nil {
		t.Fatalf("yes run: %v", err)
	}

	if _, serr := os.Stat(exe); !os.IsNotExist(serr) {
		t.Errorf("binary still exists after --yes")
	}

	if _, serr := os.Stat(lsp); !os.IsNotExist(serr) {
		t.Errorf("lsp still exists after --yes")
	}

	if !strings.Contains(out.String(), "removed ") {
		t.Errorf("output missing removals:\n%s", out.String())
	}
}

func TestRunUninstallKeepLSP(t *testing.T) {
	t.Parallel()

	exe, pathEnv := uninstallFixture(t, true)
	lsp := filepath.Join(strings.Split(pathEnv, string(os.PathListSeparator))[1], "zever-lsp")

	var out bytes.Buffer

	err := runUninstallWith(uninstallOptions{
		keepLSP:    true,
		yes:        true,
		out:        &out,
		isTerminal: func() bool { return false },
		executable: func() (string, error) { return exe, nil },
		pathEnv:    pathEnv,
		hasPathEnv: true,
		goos:       "linux",
	})
	if err != nil {
		t.Fatalf("keep-lsp run: %v", err)
	}

	if _, serr := os.Stat(lsp); serr != nil {
		t.Errorf("--keep-lsp deleted zever-lsp: %v", serr)
	}
}

func TestRunUninstallPromptDeclineAndAccept(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name     string
		answer   string
		wantErr  error
		wantGone bool
	}{
		{name: "decline", answer: "n\n", wantErr: ErrUninstallDeclined},
		{name: "empty defaults no", answer: "\n", wantErr: ErrUninstallDeclined},
		{name: "accept", answer: "yes\n", wantGone: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			exe, pathEnv := uninstallFixture(t, false)

			var out bytes.Buffer

			err := runUninstallWith(uninstallOptions{
				out:        &out,
				in:         strings.NewReader(tc.answer),
				isTerminal: func() bool { return true },
				executable: func() (string, error) { return exe, nil },
				pathEnv:    pathEnv,
				hasPathEnv: true,
				goos:       "linux",
			})

			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
			} else if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			_, serr := os.Stat(exe)
			if tc.wantGone && !os.IsNotExist(serr) {
				t.Errorf("binary still exists after confirmation")
			}

			if !tc.wantGone && serr != nil {
				t.Errorf("declined run deleted the binary: %v", serr)
			}

			if !strings.Contains(out.String(), "[y/N]") {
				t.Errorf("output missing [y/N] prompt:\n%s", out.String())
			}
		})
	}
}

func TestRunUninstallWindowsManualStep(t *testing.T) {
	t.Parallel()

	exe, pathEnv := uninstallFixture(t, false)

	var out bytes.Buffer

	err := runUninstallWith(uninstallOptions{
		dryRun:     true,
		out:        &out,
		isTerminal: func() bool { return false },
		executable: func() (string, error) { return exe, nil },
		pathEnv:    pathEnv,
		hasPathEnv: true,
		goos:       "windows",
	})
	if err != nil {
		t.Fatalf("windows dry-run: %v", err)
	}

	if !strings.Contains(out.String(), "del ") {
		t.Errorf("windows plan missing del step:\n%s", out.String())
	}

	if strings.Contains(out.String(), "would remove "+exe) {
		t.Errorf("windows plan must skip self-delete:\n%s", out.String())
	}
}

func TestRunUninstallJSONDryRun(t *testing.T) {
	var out bytes.Buffer

	var jbuf bytes.Buffer

	old := jsonOut
	jsonOut = &jbuf
	t.Cleanup(func() { jsonOut = old })

	exe, pathEnv := uninstallFixture(t, false)

	err := runUninstallWith(uninstallOptions{
		dryRun:     true,
		json:       true,
		out:        &out,
		isTerminal: func() bool { return false },
		executable: func() (string, error) { return exe, nil },
		pathEnv:    pathEnv,
		hasPathEnv: true,
		goos:       "linux",
	})
	if err != nil {
		t.Fatalf("json dry-run: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(bytes.TrimSpace(jbuf.Bytes()), &env); err != nil {
		t.Fatalf("envelope is not JSON: %v", err)
	}

	if !env.OK || env.Command != "uninstall" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want success envelope for uninstall", env)
	}
}

func TestUninstallUpgradeWiring(t *testing.T) {
	t.Parallel()

	if !useCobra([]string{"uninstall"}) {
		t.Error("useCobra(uninstall) = false, want true")
	}

	if !useCobra([]string{"upgrade"}) {
		t.Error("useCobra(upgrade) = false, want true")
	}

	seen := map[string]bool{}
	for _, c := range agentCommands() {
		seen[c.Name] = true
	}

	if !seen["uninstall"] {
		t.Error("agent catalog missing uninstall")
	}

	if !seen["upgrade"] {
		t.Error("agent catalog missing upgrade")
	}
}
