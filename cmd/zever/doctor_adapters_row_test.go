package main

import (
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
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

// TestDoctorChecksHaveNoAdaptersRow pins the removal: with no registry to
// validate, doctor checks config plus one row per battery, and nothing else.
func TestDoctorChecksHaveNoAdaptersRow(t *testing.T) {
	t.Parallel()

	c := container.New(config.Default(), cliProviders()...)
	checks := doctorChecksFor(c, config.Default())

	if _, ok := checks["adapters"]; ok {
		t.Fatal("doctor checks contain an adapters row, want config + batteries only")
	}

	if _, ok := checks["config"]; !ok {
		t.Fatal("doctor checks missing the config row")
	}

	if _, ok := checks["db"]; !ok {
		t.Fatal("doctor checks missing the db row")
	}
}

// TestCliWiringCoversDoctorBatteries pins that every battery doctor checks
// has a compiled-in default: cliDefaultDriver is never empty and always one
// of the shipped drivers, so `zever doctor` can resolve what it checks.
func TestCliWiringCoversDoctorBatteries(t *testing.T) {
	t.Parallel()

	c := container.New(config.Default(), cliProviders()...)
	checks := doctorChecksFor(c, config.Default())

	for name := range checks {
		if name == "config" || name == "job" {
			// config is the meta row; job is a dispatcher over the
			// resolved queue with no driver of its own.
			continue
		}

		def := cliDefaultDriver(name)
		if def == "" {
			t.Errorf("battery %q has no cli default driver", name)
			continue
		}

		found := false
		for _, d := range cliDriverNames(name) {
			if d == def {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("battery %q default %q not in drivers %v", name, def, cliDriverNames(name))
		}
	}
}
