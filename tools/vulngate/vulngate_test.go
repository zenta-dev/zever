package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const streamTwoFindings = `{"config": {"protocol_version": "v1.0.0"}}
{"progress": {"message": "scanning"}}
{"osv": {"id": "GO-2024-0001"}}
{"finding": {"osv": "GO-2024-0001", "fixed_version": "v1.2.3", "trace": [{"module": "example.com/mod", "version": "v1.2.2", "package": "example.com/mod/p", "function": "Vuln"}]}}
{"finding": {"osv": "GO-2024-0002", "trace": [{"module": "example.com/other", "version": "v0.9.0", "package": "example.com/other/p"}]}}
`

func testDay(t *testing.T) time.Time {
	t.Helper()
	day, err := time.Parse("2006-01-02", "2026-10-13")
	if err != nil {
		t.Fatal(err)
	}
	return day
}

func TestParseFindings(t *testing.T) {
	t.Parallel()

	findings, err := parseFindings(strings.NewReader(streamTwoFindings))
	if err != nil {
		t.Fatalf("parseFindings: %v", err)
	}
	if len(findings) != 2 {
		t.Fatalf("parseFindings found %d, want 2", len(findings))
	}
	if findings[0].ID != "GO-2024-0001" || !findings[0].Reachable {
		t.Errorf("finding[0] = %+v, want reachable GO-2024-0001", findings[0])
	}
	if findings[1].ID != "GO-2024-0002" || findings[1].Reachable {
		t.Errorf("finding[1] = %+v, want unreachable GO-2024-0002", findings[1])
	}
	if findings[0].FixedVersion != "v1.2.3" {
		t.Errorf("finding[0].FixedVersion = %q, want v1.2.3", findings[0].FixedVersion)
	}
}

func TestParseFindingsBadJSON(t *testing.T) {
	t.Parallel()

	if _, err := parseFindings(strings.NewReader("{not json")); err == nil {
		t.Error("parseFindings(bad json) = nil error, want error")
	}
}

func TestEvaluateBlocksReachable(t *testing.T) {
	t.Parallel()

	findings := []finding{{ID: "GO-2024-0001", Reachable: true}}
	res := evaluate(findings, nil, testDay(t))
	if len(res.blocked) != 1 {
		t.Fatalf("evaluate blocked %d, want 1", len(res.blocked))
	}
}

func TestEvaluateUnreachablePassesWithNote(t *testing.T) {
	t.Parallel()

	findings := []finding{{ID: "GO-2024-0002"}}
	res := evaluate(findings, nil, testDay(t))
	if len(res.blocked) != 0 {
		t.Fatalf("evaluate blocked %d unreachable, want 0", len(res.blocked))
	}
	if len(res.unreachable) != 1 {
		t.Fatalf("evaluate noted %d unreachable, want 1", len(res.unreachable))
	}
}

func TestEvaluateSuppressesAllowlisted(t *testing.T) {
	t.Parallel()

	findings := []finding{{ID: "GO-2024-0001", Reachable: true}}
	allow := map[string]allowEntry{"GO-2024-0001": {ID: "GO-2024-0001", Reason: "no fix yet, sandboxed use", Expires: "2026-12-31"}}
	res := evaluate(findings, allow, testDay(t))
	if len(res.blocked) != 0 {
		t.Fatalf("evaluate blocked allowlisted, want 0: %+v", res)
	}
	if len(res.suppressed) != 1 {
		t.Fatalf("evaluate suppressed %d, want 1", len(res.suppressed))
	}
}

func TestEvaluateExpiredFails(t *testing.T) {
	t.Parallel()

	findings := []finding{{ID: "GO-2024-0001", Reachable: true}}
	allow := map[string]allowEntry{"GO-2024-0001": {ID: "GO-2024-0001", Reason: "stale", Expires: "2026-10-01"}}
	res := evaluate(findings, allow, testDay(t))
	if len(res.expired) != 1 {
		t.Fatalf("evaluate expired %d, want 1", len(res.expired))
	}
	if len(res.blocked) != 1 {
		t.Fatalf("evaluate blocked %d with expired entry, want 1 (fail closed)", len(res.blocked))
	}
}

func TestEvaluateExpiredEntryWithoutFindingStillFails(t *testing.T) {
	t.Parallel()

	allow := map[string]allowEntry{"GO-2024-0009": {ID: "GO-2024-0009", Reason: "stale", Expires: "2026-10-01"}}
	res := evaluate(nil, allow, testDay(t))
	if len(res.expired) != 1 {
		t.Fatalf("evaluate expired %d for stale entry, want 1 (forces cleanup)", len(res.expired))
	}
}

func TestLoadAllowlistRejects(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"bad date":   `{"allowlist": [{"id": "GO-1", "reason": "r", "expires": "tomorrow"}]}`,
		"missing id": `{"allowlist": [{"reason": "r", "expires": "2026-12-31"}]}`,
		"no reason":  `{"allowlist": [{"id": "GO-1", "expires": "2026-12-31"}]}`,
		"dup id":     `{"allowlist": [{"id": "GO-1", "reason": "a", "expires": "2026-12-31"}, {"id": "GO-1", "reason": "b", "expires": "2026-12-31"}]}`,
		"bad json":   `{nope`,
	}
	for name, raw := range cases {
		if _, err := parseAllowlist(strings.NewReader(raw)); err == nil {
			t.Errorf("parseAllowlist(%s) = nil error, want error", name)
		}
	}
	if _, err := parseAllowlist(strings.NewReader(`{"allowlist": []}`)); err != nil {
		t.Errorf("parseAllowlist(empty) = %v, want nil", err)
	}
}

// stubGovulncheck writes an executable stub that cats fixture for any args,
// returning its path.
func stubGovulncheck(t *testing.T, fixture string) string {
	t.Helper()
	dir := t.TempDir()
	fixPath := filepath.Join(dir, "out.json")
	if err := os.WriteFile(fixPath, []byte(fixture), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := filepath.Join(dir, "govulncheck-stub")
	// test stub must be executable to serve as a fake govulncheck binary
	if err := os.WriteFile(stub, []byte("#!/bin/sh\ncat "+fixPath+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return stub
}

func writeAllowlist(t *testing.T, raw string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "allow.json")
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunBlocksReachableFinding(t *testing.T) {
	stub := stubGovulncheck(t, streamTwoFindings)
	allow := writeAllowlist(t, `{"allowlist": []}`)
	mod := t.TempDir()

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{
		"--root", t.TempDir(), "--allowlist", allow, "--govulncheck", stub, mod,
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run with reachable finding = exit %d, want 1\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "BLOCKED GO-2024-0001") {
		t.Errorf("output lacks BLOCKED line:\n%s", out)
	}
	if out := stdout.String(); !strings.Contains(out, "not called") {
		t.Errorf("output lacks unreachable note:\n%s", out)
	}
}

func TestRunSuppressedByAllowlist(t *testing.T) {
	stub := stubGovulncheck(t, streamTwoFindings)
	allow := writeAllowlist(t, `{"allowlist": [{"id": "GO-2024-0001", "reason": "accepted for test", "expires": "2099-01-01"}]}`)
	mod := t.TempDir()

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{
		"--root", t.TempDir(), "--allowlist", allow, "--govulncheck", stub, mod,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("run with allowlisted finding = exit %d, want 0\nstdout:\n%s\nstderr:\n%s", code, stdout.String(), stderr.String())
	}
	if out := stdout.String(); !strings.Contains(out, "suppressed GO-2024-0001") {
		t.Errorf("output lacks suppressed line:\n%s", out)
	}
}

func TestRunMissingBinaryFailsClosed(t *testing.T) {
	allow := writeAllowlist(t, `{"allowlist": []}`)

	var stdout, stderr strings.Builder
	code := run(context.Background(), []string{
		"--root", t.TempDir(), "--allowlist", allow,
		"--govulncheck", filepath.Join(t.TempDir(), "no-such-binary"), t.TempDir(),
	}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("run with missing binary = exit %d, want 1", code)
	}
}
