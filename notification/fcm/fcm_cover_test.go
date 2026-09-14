package fcm

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zenta-dev/zever/notification"
)

// writeServiceAccountJSON generates an RSA key in-test and writes a
// service-account JSON key file (0600) that the SDK can load offline.
func writeServiceAccountJSON(t *testing.T, projectID string) string {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key = %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal private key = %v", err)
	}
	pemKey := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	doc := map[string]string{ //nolint:gosec // Test fixture with freshly generated RSA key, not a real credential.
		"type":         "service_account",
		"project_id":   projectID,
		"private_key":  string(pemKey),
		"client_email": "test@" + projectID + ".iam.gserviceaccount.com",
		"token_uri":    "https://oauth2.googleapis.com/token",
	}
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal service account = %v", err)
	}
	p := filepath.Join(t.TempDir(), "sa.json")
	if err := os.WriteFile(p, raw, 0o600); err != nil {
		t.Fatalf("write service account = %v", err)
	}
	return p
}

func TestNew_garbageServiceAccount_messagingError(t *testing.T) {
	t.Parallel()

	// Passes validateServiceAccountPath (clean .json path, not a directory)
	// but fails SDK load in app.Messaging.
	p := filepath.Join(t.TempDir(), "garbage.json")
	if err := os.WriteFile(p, []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write garbage key = %v", err)
	}
	opts := notification.Options{FCM: notification.FCMOptions{ProjectID: "p", ServiceAccount: p}}
	_, err := New(opts)
	if err == nil {
		t.Fatal("New(garbage key) = nil, want load error")
	}
	if !strings.Contains(err.Error(), "fcm: init messaging") {
		t.Errorf("New(garbage key) = %v, want wrap mentioning %q", err, "fcm: init messaging")
	}
}

func TestNew_validServiceAccount_succeedsOffline(t *testing.T) {
	t.Parallel()

	p := writeServiceAccountJSON(t, "test-project")
	opts := notification.Options{FCM: notification.FCMOptions{ProjectID: "test-project", ServiceAccount: p}}
	n, err := New(opts)
	if err != nil {
		t.Fatalf("New(valid key) = %v, want nil", err)
	}
	if n == nil {
		t.Fatal("New(valid key) = nil notifier, want non-nil")
	}
	if err := n.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}

func TestNotify_realClientSend_canceledContext(t *testing.T) {
	t.Parallel()

	// Nil send means send = f.client.Send. A canceled context fails fast
	// and deterministically without any network wait.
	p := writeServiceAccountJSON(t, "test-project")
	opts := notification.Options{FCM: notification.FCMOptions{ProjectID: "test-project", ServiceAccount: p}}
	n, err := New(opts)
	if err != nil {
		t.Fatalf("New(valid key) = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := n.Notify(ctx, validPush()); err == nil {
		t.Error("Notify(canceled ctx, real client) = nil, want error")
	}
}

func TestValidateServiceAccountPath_directoryJSON(t *testing.T) {
	t.Parallel()

	// A d.json SUBDIRECTORY passes the extension check, reaching the
	// os.Stat/IsDir branch (a bare TempDir fails the earlier check).
	dir := filepath.Join(t.TempDir(), "d.json")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir d.json = %v", err)
	}
	err := validateServiceAccountPath(dir)
	if err == nil {
		t.Fatal("validateServiceAccountPath(d.json dir) = nil, want error")
	}
	if !errors.Is(err, notification.ErrInvalidOptions) {
		t.Errorf("validateServiceAccountPath(d.json dir) = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "path is a directory") {
		t.Errorf("validateServiceAccountPath(d.json dir) = %v, want %q", err, "path is a directory")
	}
}

func TestNew_directoryServiceAccount_rejects(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "d.json")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir d.json = %v", err)
	}
	opts := notification.Options{FCM: notification.FCMOptions{ProjectID: "p", ServiceAccount: dir}}
	if _, err := New(opts); !errors.Is(err, notification.ErrInvalidOptions) {
		t.Errorf("New(d.json dir) = %v, want ErrInvalidOptions", err)
	}
}
