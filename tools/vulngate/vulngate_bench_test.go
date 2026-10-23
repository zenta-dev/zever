package main

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/shared/codec"
)

// BenchmarkParseFindings measures the govulncheck JSON stream parsing.
func BenchmarkParseFindings(b *testing.B) {
	input := `{"finding":{"osv":"GO-2024-1234","fixed_version":"v1.2.3","trace":[{"function":"main.main"}]}}
{"finding":{"osv":"GO-2024-5678","fixed_version":"","trace":[{"function":"pkg.Foo"},{"function":"pkg.Bar"}]}}
{"finding":{"osv":"GO-2024-9012","fixed_version":"v2.0.0","trace":[]}}
{"finding":{"osv":"GO-2024-3456","fixed_version":"v1.1.0","trace":[{"function":"internal.Func"}]}}
{"finding":{"osv":"GO-2024-7890","fixed_version":"","trace":[]}}
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = parseFindingsFromString(input)
	}
}

// parseFindingsFromString is a copy for benchmarking.
func parseFindingsFromString(s string) ([]finding, error) {
	var findings []finding
	dec := jsontext.NewDecoder(strings.NewReader(s))
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

// BenchmarkParseAllowlist measures the allowlist JSON parsing.
func BenchmarkParseAllowlist(b *testing.B) {
	input := `{
  "allowlist": [
    {"id": "GO-2024-1234", "reason": "False positive in test code", "expires": "2025-12-31"},
    {"id": "GO-2024-5678", "reason": "Mitigated by input validation", "expires": "2025-06-15"},
    {"id": "GO-2024-9012", "reason": "Upstream fix pending", "expires": "2026-01-31"}
  ]
}
`

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = parseAllowlist(strings.NewReader(input))
	}
}

// BenchmarkEvaluate measures the finding evaluation hot path.
func BenchmarkEvaluate(b *testing.B) {
	findings := []finding{
		{ID: "GO-2024-1234", Reachable: true, FixedVersion: "v1.2.3"},
		{ID: "GO-2024-5678", Reachable: false, FixedVersion: ""},
		{ID: "GO-2024-9012", Reachable: true, FixedVersion: "v2.0.0"},
		{ID: "GO-2024-3456", Reachable: true, FixedVersion: "v1.1.0"},
		{ID: "GO-2024-7890", Reachable: false, FixedVersion: ""},
	}
	allow := map[string]allowEntry{
		"GO-2024-1234": {ID: "GO-2024-1234", Reason: "False positive", Expires: "2025-12-31"},
		"GO-2024-9012": {ID: "GO-2024-9012", Reason: "Upstream fix pending", Expires: "2026-01-31"},
	}
	today := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = evaluate(findings, allow, today)
	}
}

// BenchmarkValidDate measures the date validation hot path.
func BenchmarkValidDate(b *testing.B) {
	dates := []string{
		"2025-12-31",
		"2026-01-31",
		"2024-02-29",
		"2025-06-15",
		"invalid",
		"2025-13-01",
		"2025-00-01",
		"",
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = validDate(dates[i%len(dates)])
		i++
	}
}

// BenchmarkEvaluate_Large measures evaluation with many findings.
func BenchmarkEvaluate_Large(b *testing.B) {
	findings := make([]finding, 100)
	for i := range findings {
		findings[i] = finding{
			ID:           "GO-2024-" + string(rune('0'+i/10)) + string(rune('0'+i%10)),
			Reachable:    i%3 != 0,
			FixedVersion: "v1.0.0",
		}
	}
	allow := make(map[string]allowEntry, 30)
	for i := 0; i < 30; i++ {
		id := "GO-2024-" + string(rune('0'+i/10)) + string(rune('0'+i%10))
		allow[id] = allowEntry{
			ID:      id,
			Reason:  "Allowed",
			Expires: "2026-12-31",
		}
	}
	today := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_ = evaluate(findings, allow, today)
	}
}

// BenchmarkJSONUnmarshal measures JSON unmarshaling of finding messages.
func BenchmarkJSONUnmarshal(b *testing.B) {
	msgs := []string{
		`{"finding":{"osv":"GO-2024-1234","fixed_version":"v1.2.3","trace":[{"function":"main.main"}]}}`,
		`{"finding":{"osv":"GO-2024-5678","fixed_version":"","trace":[{"function":"pkg.Foo"},{"function":"pkg.Bar"}]}}`,
		`{"finding":{"osv":"GO-2024-9012","fixed_version":"v2.0.0","trace":[]}}`,
		`{"finding":{"osv":"GO-2024-3456","fixed_version":"v1.1.0","trace":[{"function":"internal.Func"}]}}`,
	}
	i := 0

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		var msg findingMsg
		_ = codec.Unmarshal([]byte(msgs[i%len(msgs)]), &msg)
		i++
	}
}

// BenchmarkSortExpired measures the expired entries sorting.
func BenchmarkSortExpired(b *testing.B) {
	expired := make([]allowEntry, 20)
	for i := range expired {
		expired[i] = allowEntry{
			ID:      "GO-2024-" + string(rune('0'+i/10)) + string(rune('0'+i%10)),
			Reason:  "Expired",
			Expires: "2024-01-01",
		}
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		sort.Slice(expired, func(i, j int) bool { return expired[i].ID < expired[j].ID })
	}
}
