package main

import (
	"testing"

	"github.com/zenta-dev/zever/config"
)

// TestServerDoctorRowsKeepAdaptersRow pins that the built server's adapters
// report is surfaced even though "adapters" is not an enabled battery.
func TestServerDoctorRowsKeepAdaptersRow(t *testing.T) {
	t.Parallel()

	rep := serverDoctorReport{Batteries: []serverDoctorBattery{
		{Name: "adapters", OK: false, Error: `unknown adapter "x"`},
		{Name: "db", OK: true},
		{Name: "ai", OK: true},
	}}

	rows := serverDoctorRows(config.Default(), rep, map[string]bool{"db": true})

	got := map[string]bool{}
	for _, r := range rows {
		got[r.Name] = true
	}

	if !got["adapters"] || !got["db"] || got["ai"] {
		t.Fatalf("rows = %+v, want config+adapters+db and no ai", rows)
	}
}

// TestScopedValidateAdaptersScaffoldWithUnconfiguredServices pins the
// scaffold case: only db+router enabled, nothing else registered.
func TestScopedValidateAdaptersScaffoldWithUnconfiguredServices(t *testing.T) {
	t.Parallel()

	resolved := config.Default()
	resolved.Router.Adapter = "stdhttp"

	reg := func(service string) []string {
		switch service {
		case "db":
			return []string{resolved.DB.Adapter}
		case "router":
			return []string{"stdhttp"}
		}

		return nil
	}

	if err := scopedValidateAdapters(resolved, map[string]bool{"db": true, "router": true}, reg); err != nil {
		t.Fatalf("scaffold scoped validation = %v, want nil", err)
	}
}
