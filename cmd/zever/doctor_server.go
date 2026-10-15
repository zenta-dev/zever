package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// DefaultServerDoctorTimeout bounds `go run <entry> --doctor` in runServerDoctor.
const DefaultServerDoctorTimeout = 2 * time.Minute

// serverDoctorBattery is one battery row in a built server's --doctor JSON
// document. It mirrors the CLI's doctorRow shape so reports convert without
// field mapping.
type serverDoctorBattery struct {
	// Name is the battery name, e.g. "db".
	Name string `json:"name"`
	// OK reports whether the battery resolved in the server process.
	OK bool `json:"ok"`
	// Error carries the resolution failure, empty when OK.
	Error string `json:"error,omitempty"`
}

// serverDoctorReport is the JSON document a generated server prints on
// stdout in --doctor mode (see serverTemplate's runDoctor): the aggregate
// plus one row per enabled battery.
type serverDoctorReport struct {
	// OK reports whether every enabled battery resolved.
	OK bool `json:"ok"`
	// Batteries holds one row per enabled battery.
	Batteries []serverDoctorBattery `json:"batteries"`
}

// runServerDoctor executes `go run <entry> --doctor` and parses its JSON
// report. It is a package-level variable so tests can stub the shell-out.
// Any exec or parse failure returns a non-nil error and the caller falls back
// to the CLI registry with a note.
var runServerDoctor = func(ctx context.Context, entry string) (serverDoctorReport, error) {
	ctx, cancel := context.WithTimeout(ctx, DefaultServerDoctorTimeout)
	defer cancel()

	// entry comes from the developer's own project config (server_entry),
	// not from untrusted input; argv stays separate elements, no shell.
	//nolint:gosec // see above
	cmd := exec.CommandContext(ctx, "go", buildGoRunArgs(entry, []string{"--doctor"})...)

	var stdout, stderr bytes.Buffer

	cmd.Stdout, cmd.Stderr = &stdout, &stderr

	runErr := cmd.Run()

	return interpretServerDoctor(runErr, stdout.Bytes(), stderr.String())
}

// interpretServerDoctor turns one server --doctor execution into a report. A
// server that finds a failing battery prints its JSON report and exits
// non-zero by design, so a decodable report wins over runErr; only when no
// report was printed is the run a failure, described by the last stderr line
// (the actual error, not a leading log warning).
func interpretServerDoctor(runErr error, stdout []byte, stderr string) (serverDoctorReport, error) {
	if rep, decErr := decodeServerDoctorReport(stdout); decErr == nil && len(rep.Batteries) > 0 {
		return rep, nil
	} else if runErr == nil {
		return serverDoctorReport{}, decErr
	}

	detail := "no output"
	if lines := strings.Split(strings.TrimSpace(stderr), "\n"); lines[len(lines)-1] != "" {
		detail = strings.TrimSpace(lines[len(lines)-1])
	}

	return serverDoctorReport{}, fmt.Errorf("zever doctor: server --doctor: %w: %s", runErr, detail)
}

// decodeServerDoctorReport parses one built server's --doctor JSON document,
// failing closed on malformed output.
func decodeServerDoctorReport(data []byte) (serverDoctorReport, error) {
	var rep serverDoctorReport
	if err := json.Unmarshal(bytes.TrimSpace(data), &rep); err != nil {
		return serverDoctorReport{}, fmt.Errorf("zever doctor: parse server --doctor output: %w", err)
	}

	return rep, nil
}
