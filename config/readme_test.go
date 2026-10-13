package config

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestREADMEAdapterTableMatchesConfig pins the config/README.md service table
// to the Config struct: every known service has a row, no row names an
// unknown service, and the Default() adapter for each service appears in its
// row's adapter list. Edit the table and the struct together.
func TestREADMEAdapterTableMatchesConfig(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	rows := parseREADMEAdapterTable(t, string(raw))

	if len(rows) != len(knownServiceNames()) {
		t.Errorf("README table has %d service rows, knownServiceNames has %d", len(rows), len(knownServiceNames()))
	}
	for _, name := range knownServiceNames() {
		adapters, ok := rows[name]
		if !ok {
			t.Errorf("service %q missing from README adapter table", name)
			continue
		}
		if len(adapters) == 0 {
			t.Errorf("service %q has empty adapter list in README", name)
		}
	}
	for name := range rows {
		known := false
		for _, want := range knownServiceNames() {
			if name == want {
				known = true
				break
			}
		}
		if !known {
			t.Errorf("README adapter table names unknown service %q", name)
		}
	}

	cfg := Default()
	v := reflect.ValueOf(cfg).Elem()
	typ := v.Type()
	for i := range typ.NumField() {
		name, ok := typ.Field(i).Tag.Lookup("json")
		if !ok || name == "plugins" {
			continue
		}
		adapters, ok := rows[name]
		if !ok {
			continue
		}
		def := v.Field(i).FieldByName("Adapter").String()
		listed := false
		for _, a := range adapters {
			if a == def {
				listed = true
				break
			}
		}
		if !listed {
			t.Errorf("service %q default adapter %q not listed in README row (%s)", name, def, strings.Join(adapters, ", "))
		}
	}
}

// parseREADMEAdapterTable reads the "| Service | Adapters |" markdown table
// under "## Service table" into service -> adapters.
func parseREADMEAdapterTable(t *testing.T, raw string) map[string][]string {
	t.Helper()

	rows := map[string][]string{}
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
		var adapters []string
		for _, a := range strings.Split(cells[2], ",") {
			if a = strings.TrimSpace(a); a != "" {
				adapters = append(adapters, a)
			}
		}
		rows[service] = adapters
	}
	if len(rows) == 0 {
		t.Fatalf("no service rows parsed from README adapter table")
	}
	return rows
}
