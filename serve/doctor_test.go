package serve

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"

	"github.com/zenta-dev/zever/shared/codec"
)

// mustFullProviders wires every battery the doctor report resolves: the
// boot set plus queue, auth, permission, and scheduler.
func mustFullProviders(t *testing.T, pingErr error) []container.Provider {
	t.Helper()

	return append(mustProviders(t, pingErr),
		container.Provide(queue.DefaultRef(), func(queue.Options) (queue.Queue, error) {
			return &stubQueue{}, nil
		}),
		container.Provide(auth.DefaultRef(), func(auth.Options) (auth.Auth, error) {
			return stubAuth{}, nil
		}),
		container.Provide(permission.DefaultRef(), func(permission.Options) (permission.Checker, error) {
			return stubPermission{}, nil
		}),
		container.Provide(scheduler.DefaultRef(), func(scheduler.Options) (scheduler.Scheduler, error) {
			return &stubScheduler{}, nil
		}),
	)
}

// decodeDoctorReport parses the JSON doctor document.
func decodeDoctorReport(t *testing.T, buf *bytes.Buffer) (bool, map[string]string) {
	t.Helper()

	var doc struct {
		OK        bool `json:"ok"`
		Batteries []struct {
			Name  string `json:"name"`
			OK    bool   `json:"ok"`
			Error string `json:"error"`
		} `json:"batteries"`
	}

	if err := codec.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode report: %v\n%s", err, buf.String())
	}

	rows := make(map[string]string, len(doc.Batteries))
	for _, row := range doc.Batteries {
		if !row.OK {
			rows[row.Name] = row.Error
		}
	}

	return doc.OK, rows
}

// TestDoctorReportHealthy proves the doctor report lists every battery ok.
func TestDoctorReportHealthy(t *testing.T) {
	var buf bytes.Buffer
	if err := runDoctorReport(&buf, mustFullProviders(t, nil), WithEnvOverride(config.Env{})); err != nil {
		t.Fatalf("runDoctorReport: %v", err)
	}

	ok, failing := decodeDoctorReport(t, &buf)

	if !ok {
		t.Fatalf("report ok = false: %v", failing)
	}

	if len(failing) != 0 {
		t.Fatalf("failing rows = %v, want none", failing)
	}
}

// TestDoctorReportCoversTheWiredBatteries proves the report covers the
// batteries the project wires and nothing else: an unwired battery's
// adapter package is not imported by the server binary, so resolving it
// would report "unknown adapter" -- a fact about the project, not a
// defect of the battery.
func TestDoctorReportCoversTheWiredBatteries(t *testing.T) {
	var buf bytes.Buffer
	if err := runDoctorReport(&buf, mustProviders(t, nil), WithEnvOverride(config.Env{})); err != nil {
		t.Fatalf("runDoctorReport: %v", err)
	}

	var doc struct {
		Batteries []struct {
			Name string `json:"name"`
		} `json:"batteries"`
	}

	if err := codec.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("decode report: %v\n%s", err, buf.String())
	}

	rows := make(map[string]bool, len(doc.Batteries))
	for _, row := range doc.Batteries {
		rows[row.Name] = true
	}

	for _, want := range []string{"config", "db", "log", "observability", "router"} {
		if !rows[want] {
			t.Errorf("report lacks the wired %q row: %v", want, rows)
		}
	}

	// queue, auth, permission and scheduler are not wired here.
	for _, unwanted := range []string{"queue", "auth", "permission", "scheduler"} {
		if rows[unwanted] {
			t.Errorf("report covers the unwired %q battery: %v", unwanted, rows)
		}
	}
}

// TestDoctorReportFailing proves one failing battery flips the report and
// returns ErrDoctorFailed, with the migrate hint on missing tables.
func TestDoctorReportFailing(t *testing.T) {
	t.Setenv("ZEVER_ENV", "dev")

	boom := errors.New("db: open: no such table: users")

	providers := append(
		wireStubProvidersWithDB(t, stubLogger{}, func(db.Options) (db.DB, error) {
			return nil, boom
		}),
		container.Provide(queue.DefaultRef(), func(queue.Options) (queue.Queue, error) {
			return &stubQueue{}, nil
		}),
		container.Provide(auth.DefaultRef(), func(auth.Options) (auth.Auth, error) {
			return stubAuth{}, nil
		}),
		container.Provide(permission.DefaultRef(), func(permission.Options) (permission.Checker, error) {
			return stubPermission{}, nil
		}),
		container.Provide(scheduler.DefaultRef(), func(scheduler.Options) (scheduler.Scheduler, error) {
			return &stubScheduler{}, nil
		}),
	)

	var buf bytes.Buffer
	err := runDoctorReport(&buf, providers, WithEnvOverride(config.Env{}))

	if !errors.Is(err, ErrDoctorFailed) {
		t.Fatalf("runDoctorReport err = %v, want ErrDoctorFailed", err)
	}

	ok, failing := decodeDoctorReport(t, &buf)

	if ok {
		t.Fatal("report ok = true, want false")
	}

	msg, found := failing["db"]
	if !found {
		t.Fatalf("no failing db row in %v", failing)
	}

	if !strings.Contains(msg, "zever db migrate") {
		t.Errorf("db row error = %q, want migrate hint", msg)
	}
}
