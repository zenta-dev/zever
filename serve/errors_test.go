package serve

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/authz"
)

// TestMountErrorNamesModule proves a failing Mount fails the boot naming
// the module.
func TestMountErrorNamesModule(t *testing.T) {
	boom := errors.New("boom")
	mod := &stubModule{name: "billing", mountErr: boom}

	err := runServe(context.Background(), mustProviders(t, nil), func(*container.Container) []Module {
		return []Module{mod}
	}, testOptions()...)

	if err == nil || !strings.Contains(err.Error(), `"billing"`) || !errors.Is(err, boom) {
		t.Fatalf("runServe err = %v, want mount error naming billing", err)
	}
}

// TestDuplicateModuleNamesFailClosed proves two modules sharing one name
// fail the boot before anything mounts.
func TestDuplicateModuleNamesFailClosed(t *testing.T) {
	second := &stubModule{name: "app"}

	err := runServe(context.Background(), mustProviders(t, nil), func(*container.Container) []Module {
		return []Module{&stubModule{name: "app"}, second}
	}, testOptions()...)

	if !errors.Is(err, ErrDuplicateModule) {
		t.Fatalf("runServe err = %v, want ErrDuplicateModule", err)
	}

	if second.mounted.Load() {
		t.Fatal("second module mounted despite duplicate name")
	}
}

// TestPortInUseNamesPortVar proves a busy port fails with the exact variable
// to change, never raw.
func TestPortInUseNamesPortVar(t *testing.T) {
	hold, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer func() { _ = hold.Close() }()

	_, port, err := net.SplitHostPort(hold.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort: %v", err)
	}

	err = runServe(context.Background(), mustProviders(t, nil), nil,
		WithPort(port),
		WithEnvOverride(config.Env{}),
	)

	if err == nil {
		t.Fatal("runServe = nil, want port-in-use error")
	}

	msg := err.Error()
	if !strings.Contains(msg, "port "+port+" in use") || !strings.Contains(msg, "PORT=") {
		t.Fatalf("runServe err = %q, want port hint naming PORT", msg)
	}
}

// TestDevCryptoKeyRefusedOutsideDev proves the deterministic dev key fails
// the boot outside ZEVER_ENV=dev, naming the variable.
func TestDevCryptoKeyRefusedOutsideDev(t *testing.T) {
	t.Setenv("ZEVER_ENV", "production")

	err := runServe(context.Background(), mustProvidersNoEnv(t, nil), nil, testOptions()...)

	if err == nil {
		t.Fatal("runServe = nil, want dev-key refusal")
	}

	if msg := err.Error(); !strings.Contains(msg, "ZEVER_ENV") || !strings.Contains(msg, "CRYPTO_KEY") {
		t.Fatalf("runServe err = %q, want ZEVER_ENV and CRYPTO_KEY named", msg)
	}
}

// TestGRPCPoliciesWithoutAddrFailClosed proves policy-declaring modules need
// a gRPC address.
func TestGRPCPoliciesWithoutAddrFailClosed(t *testing.T) {
	mod := &stubGRPCModule{
		stubModule: stubModule{name: "app"},
		policies:   map[string]authz.Policy{"/app.v1.App/Get": {}},
	}

	err := runServe(context.Background(), mustProviders(t, nil), func(*container.Container) []Module {
		return []Module{mod}
	}, testOptions()...)

	if !errors.Is(err, ErrGRPCPoliciesWithoutAddr) {
		t.Fatalf("runServe err = %v, want ErrGRPCPoliciesWithoutAddr", err)
	}
}

// TestNoSuchTableHintsMigrate proves a missing-table DB failure at the
// boundary names the migrate command, and other errors pass through with
// serve context only.
func TestNoSuchTableHintsMigrate(t *testing.T) {
	missing := errors.New("db: query users: no such table: users")

	if got := withMigrateHint(missing); !strings.Contains(got.Error(), "zever db migrate") || !errors.Is(got, missing) {
		t.Fatalf("withMigrateHint = %v, want migrate hint wrapping the cause", got)
	}

	other := errors.New("db: ping: connection refused")
	if got := withMigrateHint(other); !errors.Is(got, other) || strings.Contains(got.Error(), "zever db migrate") {
		t.Fatalf("withMigrateHint = %v, want passthrough", got)
	}

	if withMigrateHint(nil) != nil {
		t.Fatal("withMigrateHint(nil) != nil")
	}
}
