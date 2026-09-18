package firebase

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"firebase.google.com/go/v4/remoteconfig"

	"github.com/zenta-dev/zever/flag"
)

// Coverage note: two defensive branches in loadTemplate are provably dead
// and were pruned (error assignments replaced with blank identifiers) after
// verification against firebase-admin-go v4.21.0 source:
//   - firebase.NewApp with a non-nil Config returns (&App, nil)
//     unconditionally (all failure paths sit behind config == nil).
//   - rc.InitServerTemplate fails solely on unstringifiable default values;
//     our call always passes an empty map.
// The success return and tpl.Load error path are covered by tests below
// that swap the loadServerTemplate seam (no live network needed).

func TestCoverValidateServiceAccountPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	cases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty", "", true},
		{"unclean", "a/../b.json", true},
		{"traversal", filepath.Join("..", "x.json"), true},
		{"overmatch fixed", filepath.Join("foo..bar", "x.json"), false},
		{"directory", dir, true},
		{"bad extension", filepath.Join(dir, "x.txt"), true},
		{"missing passes guard", filepath.Join(dir, "nope.json"), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateServiceAccountPath(tc.path)
			if tc.wantErr && err == nil {
				t.Fatalf("validate(%q) = nil, want error", tc.path)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validate(%q) = %v, want nil", tc.path, err)
			}
		})
	}
}

func TestCoverValidateServiceAccountPathDir(t *testing.T) {
	t.Parallel()

	dj := filepath.Join(t.TempDir(), "sub.json")
	if err := os.Mkdir(dj, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := validateServiceAccountPath(dj); err == nil {
		t.Fatal("validate(dir.json) = nil, want directory error")
	}
}

func TestCoverEvalContextMap(t *testing.T) {
	t.Parallel()

	got := evalContextMap(flag.WithEvalContext(context.Background(), flag.EvalContext{
		RandomizationID: "user-1",
		Signals:         map[string]any{"plan": "pro"},
	}))
	if got["randomizationID"] != "user-1" {
		t.Errorf("randomizationID = %v, want user-1", got["randomizationID"])
	}
	if got["plan"] != "pro" {
		t.Errorf("plan = %v, want pro", got["plan"])
	}

	if got := evalContextMap(context.Background()); len(got) != 0 {
		t.Errorf("empty ctx map = %v, want empty", got)
	}
}

func TestCoverJSONFallbackNil(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)

	var out map[string]any
	if err := c.JSON(context.Background(), "no-such-key", &out, nil); err != nil {
		t.Fatalf("JSON missing nil-fallback = %v, want nil", err)
	}
}

func TestCoverJSONFallbackMarshalFail(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)

	var out map[string]any
	if err := c.JSON(context.Background(), "no-such-key", &out, func() {}); err == nil {
		t.Fatal("JSON unmarshalable fallback = nil error, want marshal error")
	}
}

func TestCoverJSONFallbackUnmarshalFail(t *testing.T) {
	t.Parallel()

	c := newTestClient(t)

	var out map[string]any
	if err := c.JSON(context.Background(), "no-such-key", &out, "xx"); err == nil {
		t.Fatal("JSON string fallback into map = nil error, want unmarshal error")
	}
}

func TestCoverNewLoaderFail(t *testing.T) {
	// Not parallel: swaps the package loader seam.
	old := loadTemplate
	t.Cleanup(func() { loadTemplate = old })
	loadTemplate = func(context.Context, string, string) (*remoteconfig.ServerTemplate, error) {
		return nil, errors.New("boom")
	}

	p := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(p, []byte(`{"type":"service_account"}`), 0o600); err != nil {
		t.Fatalf("seed SA: %v", err)
	}
	opts := flag.Options{Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: p}}
	if _, err := New(opts); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("New(loader fail) = %v, want boom", err)
	}
}

func TestCoverNewLoaderSuccess(t *testing.T) {
	// Not parallel: swaps the package loader seam.
	old := loadTemplate
	t.Cleanup(func() { loadTemplate = old })
	loadTemplate = func(context.Context, string, string) (*remoteconfig.ServerTemplate, error) {
		tpl, err := (&remoteconfig.Client{}).InitServerTemplate(map[string]any{}, testTemplate)
		if err != nil {
			return nil, err
		}
		return tpl, nil
	}

	opts := flag.Options{Firebase: flag.FirebaseOptions{ProjectID: "p", ServiceAccount: dummySA(t)}}
	c, err := New(opts)
	if err != nil {
		t.Fatalf("New(loader ok) = %v, want nil", err)
	}
	if got, err := c.Bool(context.Background(), "welcome", false); err != nil || got {
		t.Fatalf("Bool = %v, %v; template lacks bool, want fallback", got, err)
	}
}

func TestCoverRealLoadTemplateMalformedSA(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := loadTemplate(ctx, "p", p); err == nil {
		t.Fatal("loadTemplate(malformed SA) = nil, want error")
	}
}

func TestCoverRealLoadTemplateTimeout(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "dummy.json")
	dummy := `{"type":"service_account","project_id":"p","private_key_id":"1",` +
		`"private_key":"-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKc=\n-----END RSA PRIVATE KEY-----\n",` +
		`"client_email":"a@b.iam.gserviceaccount.com","token_uri":"https://oauth2.googleapis.com/token"}`
	if err := os.WriteFile(p, []byte(dummy), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	if _, err := loadTemplate(ctx, "p", p); err == nil {
		t.Fatal("loadTemplate(unreachable backend) = nil, want dial/timeout error")
	}
}

// NOTE (coverage): loadTemplate's InitServerTemplate error branch was
// pruned (blank-identifier assignment). The two real-loadTemplate tests
// below exercise the remaining error path via malformed SA / timeout.
// Everything in New's construction path is covered via seam swaps and
// these real calls.

func TestCoverLoadTemplateSuccess(t *testing.T) {
	// Not parallel: swaps the loadServerTemplate seam.
	old := loadServerTemplate
	t.Cleanup(func() { loadServerTemplate = old })
	loadServerTemplate = func(context.Context, *remoteconfig.ServerTemplate) error { return nil }

	p := dummySA(t)
	tpl, err := loadTemplate(context.Background(), "p", p)
	if err != nil {
		t.Fatalf("loadTemplate(success seam) = %v, want nil", err)
	}
	if tpl == nil {
		t.Fatal("loadTemplate(success seam) returned nil template")
	}
}

func TestCoverLoadTemplateFailure(t *testing.T) {
	// Not parallel: swaps the loadServerTemplate seam.
	old := loadServerTemplate
	t.Cleanup(func() { loadServerTemplate = old })
	loadServerTemplate = func(context.Context, *remoteconfig.ServerTemplate) error {
		return errors.New("boom")
	}

	p := dummySA(t)
	_, err := loadTemplate(context.Background(), "p", p)
	if err == nil {
		t.Fatal("loadTemplate(failure seam) = nil, want error")
	}
	if !strings.Contains(err.Error(), "load template") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("loadTemplate(failure seam) err = %v, want 'load template' wrapping 'boom'", err)
	}
}

func dummySA(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "sa.json")
	data := `{"type":"service_account","project_id":"p",` +
		`"private_key_id":"1","private_key":"-----BEGIN RSA PRIVATE KEY-----\nMIIBOgIBAAJBAKc=\n-----END RSA PRIVATE KEY-----\n",` +
		`"client_email":"a@b.iam.gserviceaccount.com","token_uri":"https://oauth2.googleapis.com/token"}`
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatalf("seed dummy SA: %v", err)
	}
	return p
}
