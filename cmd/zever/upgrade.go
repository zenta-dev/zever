package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

// ErrUpgradeNeedsConfirm is returned when upgrade runs without a TTY and
// without --yes/--force.
var ErrUpgradeNeedsConfirm = errors.New("zever: upgrade needs confirmation")

// ErrUpgradeDowngrade is returned when the target version is older than the
// installed one and no explicit --version requested it.
var ErrUpgradeDowngrade = errors.New("zever: upgrade would downgrade")

// ErrUpgradeChecksumMismatch is returned when a downloaded asset does not
// match its SHA256SUMS.txt entry.
var ErrUpgradeChecksumMismatch = errors.New("zever: upgrade checksum mismatch")

// ErrUpgradeVersion is returned when a version string is not valid SemVer.
var ErrUpgradeVersion = errors.New("zever: upgrade invalid version")

// ErrUpgradeDeclined is returned when the operator answers anything but yes
// at the upgrade prompt.
var ErrUpgradeDeclined = errors.New("zever: upgrade declined")

// upgradeLatestAPIURL is the GitHub API endpoint for the latest stable
// zever release.
const upgradeLatestAPIURL = "https://api.github.com/repos/zenta-dev/zever/releases/latest"

// upgradeReleaseDownloadBase is the base URL for per-tag release downloads.
const upgradeReleaseDownloadBase = "https://github.com/zenta-dev/zever/releases/download"

// upgradeSumsName is the checksum file fetched alongside every asset.
const upgradeSumsName = "SHA256SUMS.txt"

// DefaultUpgradeTimeout bounds release metadata and asset downloads.
const DefaultUpgradeTimeout = 30 * time.Second

// upgradeActionCurrent reports that the installed version already matches the
// target, so there is nothing to install.
const upgradeActionCurrent = "current"

// upgradeActionUpgrade reports that the target should be installed.
const upgradeActionUpgrade = "upgrade"

// upgradeOptions carries every input runUpgradeWith needs. Out defaults to
// os.Stdout, In defaults to os.Stdin, and the func fields default to their
// production implementations so tests can inject fakes without touching
// globals or the network.
type upgradeOptions struct {
	target     string
	check      bool
	dryRun     bool
	yes        bool
	json       bool
	out        io.Writer
	in         io.Reader
	isTerminal func() bool
	goos       string
	goarch     string
	current    string
	executable func() (string, error)
	fetch      func(ctx context.Context, url string) ([]byte, error)
	install    func(ctx context.Context, data []byte, exePath, goos string) (string, error)
}

// newUpgradeCmd builds the `upgrade` command, which replaces the running
// binary with a newer release asset.
func newUpgradeCmd() *cobra.Command {
	var target string

	var check, dryRun, yes, force bool

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade the zever binary to the latest release",
		Long: `Upgrade the running zever binary to the latest stable release
(or --version X for an explicit target).

The release asset matching runtime.GOOS/runtime.GOARCH plus SHA256SUMS.txt
is downloaded from the GitHub release, verified with crypto/sha256, and
installed atomically over the running binary.`,
		Example: `  zever upgrade --check
  zever upgrade --dry-run
  zever upgrade --yes
  zever upgrade --version v0.7.0 --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpgradeWith(context.Background(), upgradeOptions{
				target: target,
				check:  check,
				dryRun: dryRun,
				yes:    yes || force,
				json:   jsonMode,
				out:    cmd.OutOrStdout(),
				in:     cmd.InOrStdin(),
			})
		},
	}

	cmd.Flags().StringVar(&target, "version", "", "explicit target version (default: latest stable release)")
	cmd.Flags().BoolVar(&check, "check", false, "print the available version without upgrading")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the upgrade plan without downloading anything")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "confirm without prompting (non-interactive)")
	cmd.Flags().BoolVar(&force, "force", false, "alias for --yes")

	return cmd
}

// runUpgradeWith resolves the target version, compares it with the installed
// one via golang.org/x/mod/semver, and --check prints the available version,
// --dry-run prints the plan, otherwise the asset is downloaded, verified, and
// installed. Confirmation follows the same contract as `zever uninstall`.
func runUpgradeWith(ctx context.Context, opts upgradeOptions) error {
	out := outOrStdout(opts.out)

	exeFn := opts.executable
	if exeFn == nil {
		exeFn = os.Executable
	}

	exe, err := exeFn()
	if err != nil {
		return fmt.Errorf("zever upgrade: locate executable: %w", err)
	}

	if resolved, rerr := filepath.EvalSymlinks(exe); rerr == nil {
		exe = resolved
	}

	goos := opts.goos
	if goos == "" {
		goos = runtime.GOOS
	}

	goarch := opts.goarch
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	current := opts.current
	if current == "" {
		current = cliVersion
	}

	fetch := opts.fetch
	if fetch == nil {
		fetch = fetchUpgradeURL
	}

	explicit := strings.TrimSpace(opts.target) != ""

	var tag string

	assetURLs := map[string]string{}

	if explicit {
		tag = ensureUpgradeTag(opts.target)
		if !semver.IsValid(tag) {
			return fmt.Errorf("%w: %q", ErrUpgradeVersion, opts.target)
		}
	} else {
		body, ferr := fetch(ctx, upgradeLatestAPIURL)
		if ferr != nil {
			return fmt.Errorf("zever upgrade: fetch latest release: %w", ferr)
		}

		release, perr := parseUpgradeRelease(body)
		if perr != nil {
			return perr
		}

		tag = ensureUpgradeTag(release.tag)
		if !semver.IsValid(tag) {
			return fmt.Errorf("%w: %q", ErrUpgradeVersion, release.tag)
		}

		assetURLs = release.assets
	}

	action, derr := decideUpgradeTarget(current, tag, explicit)
	if derr != nil {
		return derr
	}

	asset := upgradeAssetName(goos, goarch)
	binURL, sumsURL := upgradeDownloadURLs(tag, asset)

	if configured, ok := assetURLs[asset]; ok && configured != "" {
		binURL = configured
	}

	if action == upgradeActionCurrent {
		if opts.json {
			emitSuccess("upgrade", map[string]any{
				"current": ensureUpgradeTag(current),
				"target":  tag,
				"action":  upgradeActionCurrent,
			})
		} else {
			_, _ = fmt.Fprintln(out, "already current: "+tag)
		}

		return nil
	}

	if opts.check {
		if opts.json {
			emitSuccess("upgrade", map[string]any{
				"current":   ensureUpgradeTag(current),
				"available": tag,
				"check":     true,
			})
		} else {
			_, _ = fmt.Fprintf(out, "%s available (installed %s)\n", tag, ensureUpgradeTag(current))
		}

		return nil
	}

	if opts.dryRun {
		_, _ = fmt.Fprintln(out, "Upgrade plan (binary only; projects, caches, profiles, and config are untouched):")
		_, _ = fmt.Fprintln(out, "  current "+ensureUpgradeTag(current)+" -> target "+tag)
		_, _ = fmt.Fprintln(out, "  asset "+asset)
		_, _ = fmt.Fprintln(out, "  download "+binURL)
		_, _ = fmt.Fprintln(out, "  verify "+sumsURL)

		if opts.json {
			emitSuccess("upgrade", map[string]any{
				"current": ensureUpgradeTag(current),
				"target":  tag,
				"asset":   asset,
				"dryRun":  true,
			})
		}

		return nil
	}

	if !opts.yes {
		terminal := isStdinTerminal()
		if opts.isTerminal != nil {
			terminal = opts.isTerminal()
		}

		if !terminal {
			rerun := upgradeRerunCommand(explicit, strings.TrimSpace(opts.target))

			return fmt.Errorf("%w (non-interactive): re-run with `%s`", ErrUpgradeNeedsConfirm, rerun)
		}

		stdin := opts.in
		if stdin == nil {
			stdin = os.Stdin
		}

		ok, cerr := askUpgradeConfirm(stdin, out, tag)
		if cerr != nil {
			return cerr
		}

		if !ok {
			return ErrUpgradeDeclined
		}
	}

	binData, ferr := fetch(ctx, binURL)
	if ferr != nil {
		return fmt.Errorf("zever upgrade: download %q: %w", asset, ferr)
	}

	sumsData, ferr := fetch(ctx, sumsURL)
	if ferr != nil {
		return fmt.Errorf("zever upgrade: download %q: %w", upgradeSumsName, ferr)
	}

	if verr := verifyUpgradeChecksum(binData, string(sumsData), asset); verr != nil {
		return verr
	}

	install := opts.install
	if install == nil {
		install = installUpgradeBinary
	}

	note, ierr := install(ctx, binData, exe, goos)
	if ierr != nil {
		return ierr
	}

	if opts.json {
		emitSuccess("upgrade", map[string]any{
			"current": ensureUpgradeTag(current),
			"target":  tag,
			"asset":   asset,
			"note":    note,
		})
	} else {
		_, _ = fmt.Fprintln(out, "upgraded to "+tag)
	}

	if note != "" && !opts.json {
		_, _ = fmt.Fprintln(out, note)
	}

	return nil
}

// ensureUpgradeTag normalizes a version for semver comparison and display:
// surrounding spaces are trimmed and a single leading "v" is enforced.
func ensureUpgradeTag(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return v
	}

	if strings.HasPrefix(v, "v") {
		return v
	}

	return "v" + v
}

// decideUpgradeTarget compares the installed version with the target using
// golang.org/x/mod/semver. Equal versions report upgradeActionCurrent; a
// newer installed version refuses the downgrade unless the target came from
// an explicit --version.
func decideUpgradeTarget(current, target string, explicit bool) (string, error) {
	from := ensureUpgradeTag(current)
	to := ensureUpgradeTag(target)

	if !semver.IsValid(from) {
		return "", fmt.Errorf("%w: %q", ErrUpgradeVersion, current)
	}

	if !semver.IsValid(to) {
		return "", fmt.Errorf("%w: %q", ErrUpgradeVersion, target)
	}

	cmp := semver.Compare(from, to)
	if cmp == 0 {
		return upgradeActionCurrent, nil
	}

	if cmp > 0 && !explicit {
		return "", fmt.Errorf("%w: installed %s is newer than %s (pass --version to force)", ErrUpgradeDowngrade, from, to)
	}

	return upgradeActionUpgrade, nil
}

// upgradeAssetName reports the release asset for a GOOS/GOARCH pair, adding
// the .exe suffix on Windows.
func upgradeAssetName(goos, goarch string) string {
	name := "zever-" + goos + "-" + goarch
	if goos == "windows" {
		name += ".exe"
	}

	return name
}

// upgradeDownloadURLs builds the binary and checksum URLs for a release tag
// and asset name.
func upgradeDownloadURLs(tag, asset string) (binURL, sumsURL string) {
	base := strings.TrimSuffix(upgradeReleaseDownloadBase, "/") + "/" + tag

	return base + "/" + asset, base + "/" + upgradeSumsName
}

// upgradeRerunCommand returns the exact non-interactive re-run for the
// non-TTY refusal error.
func upgradeRerunCommand(explicit bool, target string) string {
	if explicit {
		return "zever upgrade --version " + target + " --yes"
	}

	return "zever upgrade --yes"
}

// parseUpgradeAnswer reports whether a confirmation line means yes. Empty
// input (the default) means No.
func parseUpgradeAnswer(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// askUpgradeConfirm prints a [y/N] prompt for the target tag and reports
// whether the operator confirmed.
func askUpgradeConfirm(r io.Reader, w io.Writer, tag string) (bool, error) {
	_, _ = fmt.Fprintf(w, "Upgrade to %s? [y/N]: ", tag)

	scanner := bufio.NewScanner(r)

	var line string

	if scanner.Scan() {
		line = scanner.Text()
	} else if serr := scanner.Err(); serr != nil {
		return false, fmt.Errorf("zever upgrade: read confirmation: %w", serr)
	}

	return parseUpgradeAnswer(line), nil
}

// upgradeReleaseInfo is the parsed subset of a GitHub release response used
// to resolve the latest tag and per-asset download URLs.
type upgradeReleaseInfo struct {
	tag    string
	assets map[string]string
}

// parseUpgradeRelease parses a GitHub releases API response body into the
// release tag and a name-to-download-URL map.
func parseUpgradeRelease(body []byte) (upgradeReleaseInfo, error) {
	var raw struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return upgradeReleaseInfo{}, fmt.Errorf("zever upgrade: parse release metadata: %w", err)
	}

	if strings.TrimSpace(raw.TagName) == "" {
		return upgradeReleaseInfo{}, fmt.Errorf("%w: %s", ErrUpgradeVersion, "empty tag_name in release metadata")
	}

	info := upgradeReleaseInfo{tag: raw.TagName, assets: map[string]string{}}
	for _, a := range raw.Assets {
		if a.Name == "" || a.URL == "" {
			continue
		}

		info.assets[a.Name] = a.URL
	}

	return info, nil
}

// verifyUpgradeChecksum checks data against the asset's SHA256SUMS.txt entry
// using crypto/sha256.
func verifyUpgradeChecksum(data []byte, sumsText, asset string) error {
	sum := sha256.Sum256(data)
	want := ""

	for line := range strings.Lines(sumsText) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		name := strings.TrimPrefix(fields[1], "*")
		if name != asset {
			continue
		}

		want = strings.ToLower(fields[0])

		break
	}

	if want == "" {
		return fmt.Errorf("%w: no checksum entry for %q", ErrUpgradeChecksumMismatch, asset)
	}

	got := hex.EncodeToString(sum[:])
	if got != want {
		joined := errors.Join(ErrUpgradeChecksumMismatch, fmt.Errorf("zever upgrade: %q does not match %s", asset, upgradeSumsName))

		return joined
	}

	return nil
}

// fetchUpgradeURL downloads url with a bounded timeout, failing on
// non-200 statuses and network errors.
func fetchUpgradeURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("zever upgrade: build request: %w", err)
	}

	client := &http.Client{Timeout: DefaultUpgradeTimeout}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("zever upgrade: GET %s: %w", url, err)
	}

	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zever upgrade: GET %s: unexpected status %s", url, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("zever upgrade: read %s: %w", url, err)
	}

	return data, nil
}

// installUpgradeBinary installs data over exePath. POSIX installs atomically
// via temp-file plus rename; Windows stages zever.new.exe and spawns a helper
// because the running binary cannot replace itself.
func installUpgradeBinary(ctx context.Context, data []byte, exePath, goos string) (string, error) {
	if goos == "windows" {
		staged, err := stageUpgradeWindows(data, exePath)
		if err != nil {
			return "", err
		}

		if err := spawnUpgradeHelperWindows(ctx, staged, exePath); err != nil {
			return "", err
		}

		return "restart your shell to complete the upgrade", nil
	}

	if err := installUpgradePOSIX(data, exePath); err != nil {
		return "", err
	}

	return "", nil
}

// installUpgradePOSIX writes data to a temp file in the binary directory,
// marks it executable, and renames it over exePath atomically.
func installUpgradePOSIX(data []byte, exePath string) error {
	tmp, err := os.CreateTemp(filepath.Dir(exePath), ".zever-upgrade-*")
	if err != nil {
		return fmt.Errorf("zever upgrade: stage new binary: %w", err)
	}

	tmpName := tmp.Name()

	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("zever upgrade: stage new binary: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("zever upgrade: stage new binary: %w", err)
	}

	if err := os.Chmod(tmpName, 0o755); err != nil {
		return fmt.Errorf("zever upgrade: stage new binary: %w", err)
	}

	if err := os.Rename(tmpName, exePath); err != nil {
		return fmt.Errorf("zever upgrade: replace binary: %w", err)
	}

	return nil
}

// stageUpgradeWindows writes data to zever.new.exe beside exePath.
func stageUpgradeWindows(data []byte, exePath string) (string, error) {
	staged := strings.TrimSuffix(exePath, filepath.Ext(exePath)) + ".new.exe"

	if err := os.WriteFile(staged, data, 0o755); err != nil { //nolint:gosec // staged path derives from the running binary location and must stay executable.
		return "", fmt.Errorf("zever upgrade: stage new binary: %w", err)
	}

	return staged, nil
}

// spawnUpgradeHelperWindows starts a detached cmd /C helper that waits for
// our PID to exit and then moves the staged binary into place.
func spawnUpgradeHelperWindows(ctx context.Context, staged, exePath string) error {
	pid := os.Getpid()
	script := ":wait\n" + `tasklist /FI "PID eq ` + strconv.Itoa(pid) + `" 2>NUL | find "` +
		strconv.Itoa(pid) + `" >NUL && (timeout /t 1 /nobreak >NUL & goto wait)` +
		"\nmove /Y " + fmt.Sprintf("%q %q", staged, exePath)

	cmd := exec.CommandContext(ctx, "cmd", "/C", script) //nolint:gosec // staged/exePath derive from the running binary path, never remote input.

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("zever upgrade: spawn Windows helper: %w", err)
	}

	return nil
}
