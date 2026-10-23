package config

import (
	"os"
	"strings"
	"testing"
)

// TestREADMEServiceTableMatchesConfig pins the config/README.md service table
// to the Config struct: every known service has a row, and no row names an
// unknown service. The table lists services only: backend selection is
// compile-time, so the README carries no adapter matrix. Edit the table and
// the struct together.
func TestREADMEServiceTableMatchesConfig(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	rows := parseREADMEServiceTable(t, string(raw))

	if len(rows) != len(knownServiceNames()) {
		t.Errorf("README table has %d service rows, knownServiceNames has %d", len(rows), len(knownServiceNames()))
	}
	for _, name := range knownServiceNames() {
		found := false
		for _, row := range rows {
			if row == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("service %q missing from README service table", name)
		}
	}
	for _, row := range rows {
		known := false
		for _, want := range knownServiceNames() {
			if row == want {
				known = true
				break
			}
		}
		if !known {
			t.Errorf("README service table names unknown service %q", row)
		}
	}
}

// parseREADMEServiceTable reads the single-column "| Service |" markdown
// table under "## Service table" into the listed service names.
func parseREADMEServiceTable(t *testing.T, raw string) []string {
	t.Helper()

	var rows []string
	lines := strings.Split(raw, "\n")
	inTable := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inTable {
			if trimmed == "## Service table" {
				inTable = true
			}
			continue
		}
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		if strings.Contains(trimmed, "---") {
			continue
		}
		cells := strings.Split(trimmed, "|")
		if len(cells) < 3 {
			continue
		}
		service := strings.TrimSpace(cells[1])
		if service == "Service" {
			continue
		}
		if service != "" {
			rows = append(rows, service)
		}
	}
	if len(rows) == 0 {
		t.Fatalf("no service rows parsed from README service table")
	}
	return rows
}
