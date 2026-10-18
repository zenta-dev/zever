package serve

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/zenta-dev/zever/container"
)

// doctorBattery is one row of the doctor JSON document.
type doctorBattery struct {
	Name  string ``
	OK    bool   ``
	Error string ``
}

// Doctor resolves every boot battery through a real container and prints
// one JSON document ({"ok": bool, "batteries": [...]}) to stdout. It
// returns nil when every battery resolves, or ErrDoctorFailed otherwise.
// It is the programmatic form of the `doctor`/`--doctor` Main dispatch:
// the zever CLI execs the built server and parses this shape.
func Doctor(providers []container.Provider, opts ...Option) error {
	return runDoctorReport(os.Stdout, providers, opts...)
}

// runDoctorReport resolves every boot battery through a real container and
// prints one JSON document ({"ok": bool, "batteries": [...]}) to w. It
// mirrors the built server's --doctor mode: the zever CLI execs the server
// and parses this shape. It returns nil when every battery resolves, or
// ErrDoctorFailed wrapping the per-battery errors otherwise.
func runDoctorReport(w io.Writer, providers []container.Provider, opts ...Option) error {
	o := foldOptions(opts)

	cfg, err := resolveEnvConfig(o)
	if err != nil {
		return writeDoctorReport(w, false, []doctorBattery{{Name: "config", Error: err.Error()}})
	}

	c := container.New(cfg, providers...)

	checks := map[string]func() error{
		"config":        cfg.Validate,
		"log":           func() error { _, err := c.Log(); return err },
		"router":        func() error { _, err := c.Router(); return err },
		"db":            func() error { _, err := c.DB(); return withMigrateHint(err) },
		"observability": func() error { _, err := c.Observability(); return err },
		"queue":         func() error { _, err := c.Queue(); return err },
		"auth":          func() error { _, err := c.Auth(); return err },
		"permission":    func() error { _, err := c.Permission(); return err },
		"scheduler":     func() error { _, err := c.Scheduler(); return err },
	}

	names := make([]string, 0, len(checks))
	for name := range checks {
		names = append(names, name)
	}

	sort.Strings(names)

	rows := make([]doctorBattery, 0, len(names))
	ok := true

	for _, name := range names {
		row := doctorBattery{Name: name, OK: true}

		if err := checks[name](); err != nil {
			ok = false
			row.OK = false
			row.Error = err.Error()
		}

		rows = append(rows, row)
	}

	return writeDoctorReport(w, ok, rows)
}

// writeDoctorReport encodes the report and maps a failing aggregate to
// ErrDoctorFailed.
func writeDoctorReport(w io.Writer, ok bool, rows []doctorBattery) error {
	if err := json.NewEncoder(w).Encode(map[string]any{"ok": ok, "batteries": rows}); err != nil {
		return fmt.Errorf("serve: encode doctor report: %w", err)
	}

	if !ok {
		return ErrDoctorFailed
	}

	return nil
}
