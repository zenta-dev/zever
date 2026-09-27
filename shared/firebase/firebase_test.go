package firebase

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

var errBoom = errors.New("boom")

func writeJSON(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp key = %v", err)
	}
	return p
}

func TestValidate_missingProject(t *testing.T) {
	t.Parallel()
	p := writeJSON(t, `{"type":"service_account"}`)
	if err := (Credentials{ServiceAccount: p}).Validate(); err == nil {
		t.Fatal("Validate(empty project) = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "project") {
		t.Errorf("Validate(empty project) = %v, want mention of project", err)
	}
}

func TestValidate_missingServiceAccount(t *testing.T) {
	t.Parallel()
	if err := (Credentials{ProjectID: "p"}).Validate(); err == nil {
		t.Fatal("Validate(empty service account) = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "service_account") {
		t.Errorf("Validate(empty sa) = %v, want mention of service_account", err)
	}
}

func TestValidate_badPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"traversal": filepath.Join(dir, "..", "sa.json"),
		"unclean":   dir + "/./sa.json",
		"wrong ext": filepath.Join(dir, "sa.key"),
		"directory": sub,
	}
	for name, p := range cases {
		if err := (Credentials{ProjectID: "p", ServiceAccount: p}).Validate(); err == nil {
			t.Errorf("Validate(%s) = nil, want non-nil", name)
		}
	}
}

func TestValidate_invalidJSON(t *testing.T) {
	t.Parallel()
	p := writeJSON(t, "{not json")
	if err := (Credentials{ProjectID: "p", ServiceAccount: p}).Validate(); err == nil {
		t.Fatal("Validate(garbage json) = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "JSON") {
		t.Errorf("Validate(garbage json) = %v, want mention of JSON", err)
	}
}

func TestValidate_neverLogsCredentialBytes(t *testing.T) {
	t.Parallel()
	secret := "super-secret-bytes-zz9x7q"
	p := writeJSON(t, "{not json "+secret)
	err := (Credentials{ProjectID: "p", ServiceAccount: p}).Validate()
	if err == nil {
		t.Fatal("Validate = nil, want non-nil")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("Validate error leaks credential bytes: %v", err)
	}
}

func TestValidate_missingFile(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "absent.json")
	if err := (Credentials{ProjectID: "p", ServiceAccount: p}).Validate(); err == nil {
		t.Fatal("Validate(missing file) = nil, want non-nil")
	}
}

func TestValidate_valid(t *testing.T) {
	t.Parallel()
	p := writeJSON(t, `{"type":"service_account"}`)
	if err := (Credentials{ProjectID: "p", ServiceAccount: p}).Validate(); err != nil {
		t.Errorf("Validate(valid) = %v, want nil", err)
	}
}

func TestValidateServiceAccountPath_missingPasses(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "absent.json")
	if err := ValidateServiceAccountPath(p); err != nil {
		t.Errorf("ValidateServiceAccountPath(missing clean) = %v, want nil", err)
	}
}

func TestValidateServiceAccountPath_rejects(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "d.json"), 0o700); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"empty":     "",
		"traversal": "../evil.json",
		"unclean":   "a/../b.json",
		"dot":       "a/./b.json",
		"wrong ext": filepath.Join(dir, "svc.key"),
		"directory": filepath.Join(dir, "d.json"),
	}
	for name, p := range cases {
		if err := ValidateServiceAccountPath(p); err == nil {
			t.Errorf("ValidateServiceAccountPath(%s) = nil, want non-nil", name)
		}
	}
}

func TestNewApp_invalidFailsBeforeSDK(t *testing.T) {
	t.Parallel()
	if _, err := NewApp(t.Context(), Credentials{}); err == nil {
		t.Fatal("NewApp(empty) = nil, want non-nil")
	}
}

func TestNewApp_validSucceedsOffline(t *testing.T) {
	t.Parallel()
	p := writeJSON(t, `{"type":"service_account"}`)
	app, err := NewApp(t.Context(), Credentials{ProjectID: "p", ServiceAccount: p})
	if err != nil {
		t.Fatalf("NewApp(valid) = %v, want nil", err)
	}
	if app == nil {
		t.Fatal("NewApp(valid) = nil app, want non-nil")
	}
}

func TestNewApp_sdkErrorWrapped(t *testing.T) {
	old := newFirebaseApp
	t.Cleanup(func() { newFirebaseApp = old })
	newFirebaseApp = func(context.Context, *firebase.Config, ...option.ClientOption) (*firebase.App, error) {
		return nil, errBoom
	}
	p := writeJSON(t, `{"type":"service_account"}`)
	if _, err := NewApp(t.Context(), Credentials{ProjectID: "p", ServiceAccount: p}); err == nil {
		t.Fatal("NewApp(sdk fail) = nil, want non-nil")
	} else if !strings.Contains(err.Error(), "init app") {
		t.Errorf("NewApp(sdk fail) = %v, want mention of init app", err)
	}
}
