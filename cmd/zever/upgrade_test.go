package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustUpgradeSums(t *testing.T, data []byte, asset string) string {
	t.Helper()

	sum := sha256.Sum256(data)

	return hex.EncodeToString(sum[:]) + "  " + asset + "\n"
}

func TestEnsureUpgradeTag(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"bare", "0.7.0", "v0.7.0"},
		{"prefixed", "v0.7.0", "v0.7.0"},
		{"padded", "  v0.7.0  ", "v0.7.0"},
		{"empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := ensureUpgradeTag(tc.input); got != tc.want {
				t.Errorf("ensureUpgradeTag(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDecideUpgradeTarget(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		current  string
		target   string
		explicit bool
		want     string
		wantErr  error
	}{
		{name: "same is current", current: "0.6.0", target: "v0.6.0", want: upgradeActionCurrent},
		{name: "newer upgrades", current: "0.6.0", target: "v0.7.0", want: upgradeActionUpgrade},
		{name: "patch upgrades", current: "v0.6.0", target: "0.6.1", want: upgradeActionUpgrade},
		{name: "older refuses downgrade", current: "0.7.0", target: "v0.6.0", wantErr: ErrUpgradeDowngrade},
		{name: "older explicit upgrades", current: "0.7.0", target: "v0.6.0", explicit: true, want: upgradeActionUpgrade},
		{name: "invalid current", current: "banana", target: "v0.7.0", wantErr: ErrUpgradeVersion},
		{name: "invalid target", current: "0.6.0", target: "banana", wantErr: ErrUpgradeVersion},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := decideUpgradeTarget(tc.current, tc.target, tc.explicit)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("err = %v, want nil", err)
			}

			if got != tc.want {
				t.Errorf("action = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestUpgradeAssetName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		goos   string
		goarch string
		want   string
	}{
		{name: "linux amd64", goos: "linux", goarch: "amd64", want: "zever-linux-amd64"},
		{name: "linux arm64", goos: "linux", goarch: "arm64", want: "zever-linux-arm64"},
		{name: "darwin arm64", goos: "darwin", goarch: "arm64", want: "zever-darwin-arm64"},
		{name: "darwin amd64", goos: "darwin", goarch: "amd64", want: "zever-darwin-amd64"},
		{name: "windows exe", goos: "windows", goarch: "amd64", want: "zever-windows-amd64.exe"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			if got := upgradeAssetName(tc.goos, tc.goarch); got != tc.want {
				t.Errorf("upgradeAssetName(%q, %q) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
			}
		})
	}
}

func TestUpgradeDownloadURLs(t *testing.T) {
	t.Parallel()

	bin, sums := upgradeDownloadURLs("v0.7.0", "zever-linux-amd64")

	if !strings.Contains(bin, "v0.7.0/zever-linux-amd64") {
		t.Errorf("bin URL %q missing tag/asset", bin)
	}

	if !strings.Contains(sums, "v0.7.0/"+upgradeSumsName) {
		t.Errorf("sums URL %q missing tag/"+upgradeSumsName, sums)
	}
}

func TestVerifyUpgradeChecksum(t *testing.T) {
	t.Parallel()

	data := []byte("fake release binary")
	asset := "zever-linux-amd64"

	if err := verifyUpgradeChecksum(data, mustUpgradeSums(t, data, asset), asset); err != nil {
		t.Errorf("valid checksum: %v", err)
	}

	star := strings.Replace(mustUpgradeSums(t, data, asset), "  ", " *", 1)
	if err := verifyUpgradeChecksum(data, star, asset); err != nil {
		t.Errorf("star-form checksum: %v", err)
	}

	if err := verifyUpgradeChecksum([]byte("tampered"), mustUpgradeSums(t, data, asset), asset); !errors.Is(err, ErrUpgradeChecksumMismatch) {
		t.Errorf("tampered err = %v, want ErrUpgradeChecksumMismatch", err)
	}

	if err := verifyUpgradeChecksum(data, mustUpgradeSums(t, data, "other-asset"), asset); !errors.Is(err, ErrUpgradeChecksumMismatch) {
		t.Errorf("missing entry err = %v, want ErrUpgradeChecksumMismatch", err)
	}

	if err := verifyUpgradeChecksum(data, "not a sums file\n", asset); !errors.Is(err, ErrUpgradeChecksumMismatch) {
		t.Errorf("garbage sums err = %v, want ErrUpgradeChecksumMismatch", err)
	}
}

func TestParseUpgradeRelease(t *testing.T) {
	t.Parallel()

	body := `{"tag_name":"v0.7.0","assets":[{"name":"zever-linux-amd64","browser_download_url":"https://example.com/zever-linux-amd64"},{"name":"SHA256SUMS.txt","browser_download_url":"https://example.com/SHA256SUMS.txt"}]}`

	info, err := parseUpgradeRelease([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if info.tag != "v0.7.0" {
		t.Errorf("tag = %q, want v0.7.0", info.tag)
	}

	if info.assets["zever-linux-amd64"] != "https://example.com/zever-linux-amd64" {
		t.Errorf("assets = %v, want the linux URL", info.assets)
	}

	if _, err := parseUpgradeRelease([]byte("{oops")); err == nil {
		t.Error("bad JSON: got nil error")
	}

	if _, err := parseUpgradeRelease([]byte(`{"tag_name":""}`)); !errors.Is(err, ErrUpgradeVersion) {
		t.Errorf("empty tag err = %v, want ErrUpgradeVersion", err)
	}
}

func TestParseUpgradeAnswer(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input string
		want  bool
	}{
		{"y", true},
		{"YES", true},
		{"  yes ", true},
		{"", false},
		{"n", false},
		{"nope", false},
	}

	for _, tc := range cases {
		if got := parseUpgradeAnswer(tc.input); got != tc.want {
			t.Errorf("parseUpgradeAnswer(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestUpgradeRerunCommand(t *testing.T) {
	t.Parallel()

	if got := upgradeRerunCommand(false, ""); got != "zever upgrade --yes" {
		t.Errorf("latest rerun = %q", got)
	}

	if got := upgradeRerunCommand(true, "v0.7.0"); !strings.Contains(got, "--version v0.7.0") || !strings.Contains(got, "--yes") {
		t.Errorf("explicit rerun = %q", got)
	}
}

const upgradeTestLatestJSON = `{"tag_name":"v0.7.0","assets":[{"name":"zever-linux-amd64","browser_download_url":"https://example.com/zever-linux-amd64"}]}`

func upgradeTestFetch(t *testing.T, bin []byte) func(context.Context, string) ([]byte, error) {
	t.Helper()

	const asset = "zever-linux-amd64"

	sums := mustUpgradeSums(t, bin, asset)

	return func(_ context.Context, url string) ([]byte, error) {
		switch {
		case strings.HasSuffix(url, "releases/latest"):
			return []byte(upgradeTestLatestJSON), nil
		case strings.HasSuffix(url, upgradeSumsName):
			return []byte(sums), nil
		default:
			return bin, nil
		}
	}
}

func TestRunUpgradeDryRunExplicit(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	fetch := func(context.Context, string) ([]byte, error) {
		t.Error("dry-run must not fetch")

		return nil, errors.New("zever: unexpected fetch")
	}
	install := func(context.Context, []byte, string, string) (string, error) {
		t.Error("dry-run must not install")

		return "", errors.New("zever: unexpected install")
	}

	err := runUpgradeWith(t.Context(), upgradeOptions{
		target:     "v0.7.0",
		dryRun:     true,
		out:        &out,
		isTerminal: func() bool { return false },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch:      fetch,
		install:    install,
	})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}

	got := out.String()
	for _, want := range []string{"v0.7.0", "zever-linux-amd64", "SHA256SUMS.txt"} {
		if !strings.Contains(got, want) {
			t.Errorf("dry-run output missing %q:\n%s", want, got)
		}
	}
}

func TestRunUpgradeCheckPrintsAvailable(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	installed := false

	err := runUpgradeWith(t.Context(), upgradeOptions{
		check:      true,
		out:        &out,
		isTerminal: func() bool { return false },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch:      upgradeTestFetch(t, []byte("bin")),
		install: func(context.Context, []byte, string, string) (string, error) {
			installed = true

			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}

	if !strings.Contains(out.String(), "v0.7.0") {
		t.Errorf("check output missing available version:\n%s", out.String())
	}

	if installed {
		t.Error("check installed a binary")
	}
}

func TestRunUpgradeAlreadyCurrent(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := runUpgradeWith(t.Context(), upgradeOptions{
		target:     "v0.6.0",
		out:        &out,
		isTerminal: func() bool { return false },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch: func(context.Context, string) ([]byte, error) {
			t.Error("already-current must not fetch")

			return nil, errors.New("zever: unexpected fetch")
		},
	})
	if err != nil {
		t.Fatalf("already-current: %v", err)
	}

	if !strings.Contains(out.String(), "already current") {
		t.Errorf("output missing already-current note:\n%s", out.String())
	}
}

func TestRunUpgradeDowngradeRefused(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := runUpgradeWith(t.Context(), upgradeOptions{
		out:        &out,
		isTerminal: func() bool { return true },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.7.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch: func(context.Context, string) ([]byte, error) {
			return []byte(`{"tag_name":"v0.6.0","assets":[]}`), nil
		},
	})
	if !errors.Is(err, ErrUpgradeDowngrade) {
		t.Fatalf("err = %v, want ErrUpgradeDowngrade", err)
	}
}

func TestRunUpgradeNeedsConfirmNonTTY(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer

	err := runUpgradeWith(t.Context(), upgradeOptions{
		target:     "v0.7.0",
		out:        &out,
		isTerminal: func() bool { return false },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch:      upgradeTestFetch(t, []byte("bin")),
	})
	if !errors.Is(err, ErrUpgradeNeedsConfirm) {
		t.Fatalf("err = %v, want ErrUpgradeNeedsConfirm", err)
	}

	if !strings.Contains(err.Error(), "zever upgrade --version v0.7.0 --yes") {
		t.Errorf("err %q missing the exact re-run command", err.Error())
	}
}

func TestRunUpgradeFullInstall(t *testing.T) {
	t.Parallel()

	want := []byte("new zever binary")

	var out bytes.Buffer

	var got []byte

	err := runUpgradeWith(t.Context(), upgradeOptions{
		target:     "v0.7.0",
		yes:        true,
		out:        &out,
		isTerminal: func() bool { return false },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch:      upgradeTestFetch(t, want),
		install: func(_ context.Context, data []byte, exePath, goos string) (string, error) {
			got = data
			if exePath != "/bin/zever" || goos != "linux" {
				t.Errorf("install(%q, %q)", exePath, goos)
			}

			return "", nil
		},
	})
	if err != nil {
		t.Fatalf("install run: %v", err)
	}

	if string(got) != string(want) {
		t.Errorf("installed %q, want %q", got, want)
	}

	if !strings.Contains(out.String(), "upgraded to v0.7.0") {
		t.Errorf("output missing upgrade note:\n%s", out.String())
	}
}

func TestRunUpgradeChecksumMismatchAborts(t *testing.T) {
	t.Parallel()

	installed := false

	err := runUpgradeWith(t.Context(), upgradeOptions{
		target:     "v0.7.0",
		yes:        true,
		out:        &bytes.Buffer{},
		isTerminal: func() bool { return false },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch: func(_ context.Context, url string) ([]byte, error) {
			if strings.HasSuffix(url, upgradeSumsName) {
				return []byte(mustUpgradeSums(t, []byte("expected"), "zever-linux-amd64")), nil
			}

			return []byte("tampered"), nil
		},
		install: func(context.Context, []byte, string, string) (string, error) {
			installed = true

			return "", nil
		},
	})
	if !errors.Is(err, ErrUpgradeChecksumMismatch) {
		t.Fatalf("err = %v, want ErrUpgradeChecksumMismatch", err)
	}

	if installed {
		t.Error("mismatched binary was installed")
	}
}

func TestRunUpgradePromptDecline(t *testing.T) {
	t.Parallel()

	err := runUpgradeWith(t.Context(), upgradeOptions{
		target:     "v0.7.0",
		out:        &bytes.Buffer{},
		in:         strings.NewReader("n\n"),
		isTerminal: func() bool { return true },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch:      upgradeTestFetch(t, []byte("bin")),
		install: func(context.Context, []byte, string, string) (string, error) {
			t.Error("declined run must not install")

			return "", nil
		},
	})
	if !errors.Is(err, ErrUpgradeDeclined) {
		t.Fatalf("err = %v, want ErrUpgradeDeclined", err)
	}
}

func TestRunUpgradeJSONCheck(t *testing.T) {
	var jbuf bytes.Buffer

	old := jsonOut
	jsonOut = &jbuf
	t.Cleanup(func() { jsonOut = old })

	err := runUpgradeWith(t.Context(), upgradeOptions{
		check:      true,
		json:       true,
		out:        &bytes.Buffer{},
		isTerminal: func() bool { return false },
		goos:       "linux",
		goarch:     "amd64",
		current:    "0.6.0",
		executable: func() (string, error) { return "/bin/zever", nil },
		fetch:      upgradeTestFetch(t, []byte("bin")),
	})
	if err != nil {
		t.Fatalf("json check: %v", err)
	}

	var env Envelope
	if err := json.Unmarshal(bytes.TrimSpace(jbuf.Bytes()), &env); err != nil {
		t.Fatalf("envelope is not JSON: %v", err)
	}

	if !env.OK || env.Command != "upgrade" || env.ExitCode != ExitOK {
		t.Fatalf("envelope = %+v, want success envelope for upgrade", env)
	}
}
