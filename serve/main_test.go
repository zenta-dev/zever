package serve

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/core/scheduler"
)

// TestMainDoctorArgWritesReport proves the `doctor` argument prints the
// JSON report to stdout and returns nil when healthy.
func TestMainDoctorArgWritesReport(t *testing.T) {
	oldArgs, oldStdout := os.Args, os.Stdout
	defer func() { os.Args, os.Stdout = oldArgs, oldStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}

	os.Args = []string{"app", "doctor"}
	os.Stdout = w

	mainErr := make(chan error, 1)

	go func() {
		mainErr <- Main(mustFullProviders(t, nil), nil, WithEnvOverride(config.Env{}))
		_ = w.Close()
	}()

	out, readErr := io.ReadAll(r)
	_ = r.Close()

	if err := <-mainErr; err != nil {
		t.Fatalf("Main doctor: %v", err)
	}

	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}

	var doc struct {
		OK        bool `json:"ok"`
		Batteries []struct {
			Name string `json:"name"`
			OK   bool   `json:"ok"`
		} `json:"batteries"`
	}

	if err := json.Unmarshal(bytes.TrimSpace(out), &doc); err != nil {
		t.Fatalf("decode report: %v\n%s", err, out)
	}

	if !doc.OK || len(doc.Batteries) == 0 {
		t.Fatalf("report not ok: %s", out)
	}
}

// TestMainDoctorArgFailsClosed proves a failing battery makes the doctor
// argument return ErrDoctorFailed.
func TestMainDoctorArgFailsClosed(t *testing.T) {
	oldArgs, oldStdout := os.Args, os.Stdout
	defer func() { os.Args, os.Stdout = oldArgs, oldStdout }()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}

	os.Args = []string{"app", "--doctor"}
	os.Stdout = w

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

	mainErr := make(chan error, 1)

	go func() {
		mainErr <- Main(providers, nil, WithEnvOverride(config.Env{}))
		_ = w.Close()
	}()

	_, _ = io.ReadAll(r)
	_ = r.Close()

	if err := <-mainErr; !errors.Is(err, ErrDoctorFailed) {
		t.Fatalf("Main doctor err = %v, want ErrDoctorFailed", err)
	}
}

// TestPlaintextWarningOnWildcard proves serving plaintext on a non-loopback
// bind warns naming TLS material.
func TestPlaintextWarningOnWildcard(t *testing.T) {
	t.Setenv("ZEVER_ENV", "dev")

	logger := &recordLogger{}

	base, shutdown := serveOnce(t, wireStubProviders(t, nil, logger), func(*container.Container) []Module {
		return nil
	}, testOptions()...)
	defer shutdown()

	if got := getStatus(t, base+"/healthz"); got != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", got)
	}

	if !logger.warnsContaining("plaintext") {
		t.Fatal("no plaintext warning on wildcard bind")
	}
}

// TestTLSBootServesHTTPSWithHSTS proves cert/key env boots an HTTPS
// listener with HSTS and no plaintext warning.
func TestTLSBootServesHTTPSWithHSTS(t *testing.T) {
	t.Setenv("ZEVER_ENV", "dev")

	dir := t.TempDir()
	certFile, keyFile := mustTLSFiles(t, dir)

	logger := &recordLogger{}
	env := config.Env{
		"ROUTER_TLS_CERT": certFile,
		"ROUTER_TLS_KEY":  keyFile,
	}

	base, shutdown := serveOnce(t, wireStubProviders(t, nil, logger), func(*container.Container) []Module {
		return nil
	}, WithPort("0"), WithEnvOverride(env))
	defer shutdown()

	tlsBase := strings.Replace(base, "http://", "https://", 1)

	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test-only loopback client.
	}}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, tlsBase+"/healthz", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz = %d, want 200", resp.StatusCode)
	}

	if got := resp.Header.Get("Strict-Transport-Security"); !strings.Contains(got, "max-age=") {
		t.Errorf("HSTS = %q with TLS, want max-age", got)
	}

	if logger.warnsContaining("plaintext") {
		t.Error("plaintext warning with TLS active")
	}
}
