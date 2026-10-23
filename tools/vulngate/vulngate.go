package main

import (
	"bytes"
	"context"
	"encoding/json/jsontext"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/zenta-dev/zever/shared/codec"
)

// finding is one govulncheck -json finding object, reduced to the fields the
// gate decides on.
type finding struct {
	// ID is the GO-... advisory identifier.
	ID string
	// Reachable reports whether the trace names a called symbol. At symbol
	// scan level a finding without a symbol was imported but never called.
	Reachable bool
	// FixedVersion names the fixing release, when the advisory has one.
	FixedVersion string
	// Module is the scanned module directory the finding came from.
	Module string
}

// allowEntry is one accepted-risk record in the allowlist file.
type allowEntry struct {
	// ID is the GO-... advisory identifier.
	ID string `json:"id"`
	// Reason explains why the risk is accepted.
	Reason string `json:"reason"`
	// Expires is the re-triage date (YYYY-MM-DD).
	Expires string `json:"expires"`
}

// verdict groups evaluated findings by gate outcome.
type verdict struct {
	blocked     []finding
	suppressed  []finding
	unreachable []finding
	expired     []allowEntry
}

// frame mirrors one govulncheck trace frame (only decided fields kept).
type frame struct {
	Function string `json:"function"`
}

// findingMsg mirrors one {"finding": {...}} stream object.
type findingMsg struct {
	Finding struct {
		OSV          string  `json:"osv"`
		FixedVersion string  `json:"fixed_version"`
		Trace        []frame `json:"trace"`
	} `json:"finding"`
}

// parseFindings decodes a govulncheck -json object stream, returning every
// finding. Config, SBOM, progress, and osv objects carry no verdict and are
// skipped; malformed JSON fails closed.
func parseFindings(r io.Reader) ([]finding, error) {
	var findings []finding
	dec := jsontext.NewDecoder(r)
	for {
		var raw jsontext.Value
		if err := codec.UnmarshalDecode(dec, &raw); err != nil {
			if errors.Is(err, io.EOF) {
				return findings, nil
			}
			return nil, fmt.Errorf("vulngate: decode govulncheck output: %w", err)
		}
		var msg findingMsg
		if err := codec.Unmarshal(raw, &msg); err != nil {
			return nil, fmt.Errorf("vulngate: decode govulncheck output: %w", err)
		}
		if msg.Finding.OSV == "" {
			continue
		}
		reachable := false
		for _, fr := range msg.Finding.Trace {
			if fr.Function != "" {
				reachable = true
				break
			}
		}
		findings = append(findings, finding{
			ID:           msg.Finding.OSV,
			Reachable:    reachable,
			FixedVersion: msg.Finding.FixedVersion,
		})
	}
}

// allowFile mirrors the allowlist JSON document.
type allowFile struct {
	Allowlist []allowEntry `json:"allowlist"`
}

// parseAllowlist decodes and validates the allowlist document: every entry
// needs an id, a reason, and a YYYY-MM-DD expiry; duplicate ids fail.
func parseAllowlist(r io.Reader) (map[string]allowEntry, error) {
	var doc allowFile
	if err := codec.UnmarshalRead(r, &doc); err != nil {
		return nil, fmt.Errorf("vulngate: parse allowlist: %w", err)
	}
	allow := make(map[string]allowEntry, len(doc.Allowlist))
	for _, e := range doc.Allowlist {
		switch {
		case e.ID == "":
			return nil, errors.New("vulngate: allowlist entry without id")
		case e.Reason == "":
			return nil, fmt.Errorf("vulngate: allowlist entry %s without reason", e.ID)
		case !validDate(e.Expires):
			return nil, fmt.Errorf("vulngate: allowlist entry %s has bad expiry %q (want YYYY-MM-DD)", e.ID, e.Expires)
		}
		if _, dup := allow[e.ID]; dup {
			return nil, fmt.Errorf("vulngate: duplicate allowlist entry %s", e.ID)
		}
		allow[e.ID] = e
	}
	return allow, nil
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// evaluate sorts findings into the gate verdict for today: reachable
// findings block unless covered by an unexpired allowlist entry;
// unreachable findings are noted but never block; expired entries fail
// closed even when their advisory is gone, forcing cleanup or re-triage.
func evaluate(findings []finding, allow map[string]allowEntry, today time.Time) verdict {
	var res verdict
	seen := map[string]bool{}
	for _, f := range findings {
		entry, listed := allow[f.ID]
		switch {
		case listed && entry.Expires >= today.Format("2006-01-02"):
			seen[f.ID] = true
			res.suppressed = append(res.suppressed, f)
		case !f.Reachable:
			res.unreachable = append(res.unreachable, f)
		default:
			res.blocked = append(res.blocked, f)
		}
	}
	for id, entry := range allow {
		if seen[id] {
			continue
		}
		if entry.Expires < today.Format("2006-01-02") {
			res.expired = append(res.expired, entry)
		}
	}
	sort.Slice(res.expired, func(i, j int) bool { return res.expired[i].ID < res.expired[j].ID })
	return res
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("vulngate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	rootFlag := fs.String("root", "", "repo root (default: git toplevel of cwd)")
	allowFlag := fs.String("allowlist", "", "allowlist path (default: <root>/.github/govulncheck-allowlist.json)")
	binFlag := fs.String("govulncheck", "govulncheck", "govulncheck binary")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(stderr, "vulngate: no module directories given")
		return 2
	}

	root := *rootFlag
	if root == "" {
		out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
		if err != nil {
			fmt.Fprintf(stderr, "vulngate: git rev-parse --show-toplevel: %v\n", err)
			return 1
		}
		root = strings.TrimSpace(string(out))
	}
	allowPath := *allowFlag
	if allowPath == "" {
		allowPath = filepath.Join(root, ".github", "govulncheck-allowlist.json")
	}
	raw, err := os.ReadFile(allowPath)
	if err != nil {
		fmt.Fprintf(stderr, "vulngate: read %s: %v\n", allowPath, err)
		return 1
	}
	allow, err := parseAllowlist(bytes.NewReader(raw))
	if err != nil {
		fmt.Fprintf(stderr, "vulngate: %v\n", err)
		return 1
	}

	var findings []finding
	if _, err := exec.LookPath(*binFlag); err != nil {
		// When --govulncheck names a path (tests), LookPath still resolves
		// it; only a truly missing binary fails here.
		if _, statErr := os.Stat(*binFlag); statErr != nil {
			fmt.Fprintf(stderr, "vulngate: govulncheck binary %q not found: %v\n", *binFlag, err)
			return 1
		}
	}
	for _, dir := range fs.Args() {
		found, err := scanModule(ctx, *binFlag, dir)
		if err != nil {
			fmt.Fprintf(stderr, "vulngate: %s: %v\n", dir, err)
			return 1
		}
		findings = append(findings, found...)
	}

	res := evaluate(findings, allow, time.Now().UTC().Truncate(24*time.Hour))
	byID := map[string][]finding{}
	for _, f := range res.blocked {
		byID[f.ID] = append(byID[f.ID], f)
	}
	for _, id := range sortedKeys(byID) {
		mods := map[string]bool{}
		fixed := ""
		for _, f := range byID[id] {
			mods[f.Module] = true
			fixed = f.FixedVersion
		}
		fmt.Fprintf(stdout, "vulngate: BLOCKED %s (fix %s) in %s\n", id, orUnknown(fixed), strings.Join(sortedKeys(mods), ", "))
	}
	for _, f := range res.unreachable {
		fmt.Fprintf(stdout, "vulngate: note %s imported but not called in %s\n", f.ID, f.Module)
	}
	for _, f := range res.suppressed {
		fmt.Fprintf(stdout, "vulngate: suppressed %s in %s (allowlisted)\n", f.ID, f.Module)
	}
	for _, e := range res.expired {
		fmt.Fprintf(stdout, "vulngate: EXPIRED allowlist entry %s (expired %s, re-triage or drop)\n", e.ID, e.Expires)
	}
	if len(res.blocked) > 0 || len(res.expired) > 0 {
		return 1
	}
	fmt.Fprintf(stdout, "vulngate: clean (%d unreachable notes, %d suppressed)\n", len(res.unreachable), len(res.suppressed))
	return 0
}

// scanModule runs govulncheck -json in dir and tags parsed findings with it.
// govulncheck exits non-zero when findings exist, so output is parsed
// regardless of the exit code; unparseable output fails the module.
func scanModule(ctx context.Context, bin, dir string) ([]finding, error) {
	cmd := exec.CommandContext(ctx, bin, "-json", "./...")
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	found, err := parseFindings(&stdout)
	if err != nil {
		return nil, fmt.Errorf("vulngate: parse %s: %w (stderr: %s)", dir, err, strings.TrimSpace(stderr.String()))
	}
	if runErr != nil && len(found) == 0 && strings.TrimSpace(stderr.String()) != "" {
		return nil, fmt.Errorf("vulngate: scan %s: %s: %w", dir, strings.TrimSpace(stderr.String()), runErr)
	}
	for i := range found {
		found[i].Module = dir
	}
	return found, nil
}

func sortedKeys[T any](m map[string]T) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
