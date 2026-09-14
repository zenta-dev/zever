package fcm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"firebase.google.com/go/v4/messaging"

	"github.com/zenta-dev/zever/notification"
)

func validPush() *notification.Notification {
	return &notification.Notification{
		Target:  "device-token-123",
		Channel: notification.ChannelPush,
		Title:   "Hello",
		Body:    "World",
	}
}

func TestNotify_mapsHighPriorityDataAndTTL(t *testing.T) {
	t.Parallel()
	var got *messaging.Message
	n := &notifier{send: func(_ context.Context, m *messaging.Message) (string, error) {
		got = m
		return "projects/p/messages/m", nil
	}}
	before := time.Now()
	in := validPush()
	in.Priority = notification.PriorityHigh
	in.TTL = time.Hour
	in.Data = map[string]string{"k": "v"}
	if err := n.Notify(context.Background(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got == nil {
		t.Fatal("fake send not called")
	}
	token := got.Token //nolint:staticcheck // Token field intentional, see fcm.go.
	if token != "device-token-123" {
		t.Errorf("Token = %q, want device token", token)
	}
	if got.Notification == nil || got.Notification.Title != "Hello" || got.Notification.Body != "World" {
		t.Errorf("Notification = %+v, want title/body", got.Notification)
	}
	if got.Data["k"] != "v" {
		t.Errorf("Data = %v, want k=v", got.Data)
	}
	if got.Android == nil || got.Android.Priority != "high" {
		t.Errorf("Android = %+v, want priority high", got.Android)
	}
	if got.APNS == nil || got.APNS.Headers["apns-priority"] != "10" {
		t.Errorf("APNS = %+v, want apns-priority 10", got.APNS)
	}
	if got.Android.TTL == nil || *got.Android.TTL != time.Hour {
		t.Errorf("Android.TTL = %v, want 1h", got.Android.TTL)
	}
	exp, err := strconv.ParseInt(got.APNS.Headers["apns-expiration"], 10, 64)
	if err != nil {
		t.Fatalf("apns-expiration = %q, not a unix int: %v", got.APNS.Headers["apns-expiration"], err)
	}
	want := before.Add(time.Hour).Unix()
	if d := exp - want; d < -120 || d > 120 {
		t.Errorf("apns-expiration = %d, want ~%d", exp, want)
	}
}

func TestNotify_mapsDefaultPriorityWithoutTTL(t *testing.T) {
	t.Parallel()
	var got *messaging.Message
	n := &notifier{send: func(_ context.Context, m *messaging.Message) (string, error) {
		got = m
		return "id", nil
	}}
	if err := n.Notify(context.Background(), validPush()); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.Android == nil || got.Android.Priority != "normal" {
		t.Errorf("Android = %+v, want priority normal", got.Android)
	}
	if got.APNS == nil || got.APNS.Headers["apns-priority"] != "5" {
		t.Errorf("APNS = %+v, want apns-priority 5", got.APNS)
	}
	if got.Android.TTL != nil {
		t.Errorf("Android.TTL = %v, want nil", *got.Android.TTL)
	}
	if _, ok := got.APNS.Headers["apns-expiration"]; ok {
		t.Errorf("APNS headers = %v, want no apns-expiration", got.APNS.Headers)
	}
	if got.Data != nil {
		t.Errorf("Data = %v, want nil", got.Data)
	}
}

func TestNotify_sendError_wrapped(t *testing.T) {
	t.Parallel()
	boom := errors.New("boom")
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		return "", boom
	}}
	err := n.Notify(context.Background(), validPush())
	if err == nil {
		t.Fatal("Notify() = nil, want send error")
	}
	if !errors.Is(err, boom) {
		t.Errorf("Notify() = %v, want wrap of boom", err)
	}
}

func TestNotify_nil_rejects(t *testing.T) {
	t.Parallel()
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		return "id", nil
	}}
	if err := n.Notify(context.Background(), nil); !errors.Is(err, notification.ErrNilNotification) {
		t.Errorf("Notify(nil) = %v, want ErrNilNotification", err)
	}
}

func TestNotify_wrongChannel_rejects(t *testing.T) {
	t.Parallel()
	called := false
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		called = true
		return "id", nil
	}}
	in := &notification.Notification{
		Target:  "+15551234567",
		Channel: notification.ChannelSMS,
		Body:    "hi",
	}
	if err := n.Notify(context.Background(), in); !errors.Is(err, notification.ErrChannelNotSupported) {
		t.Errorf("Notify(sms) = %v, want ErrChannelNotSupported", err)
	}
	if called {
		t.Error("send called for unsupported channel")
	}
}

func TestNotify_invalidNotification_rejects(t *testing.T) {
	t.Parallel()
	called := false
	n := &notifier{send: func(_ context.Context, _ *messaging.Message) (string, error) {
		called = true
		return "id", nil
	}}
	in := validPush()
	in.Target = ""
	if err := n.Notify(context.Background(), in); !errors.Is(err, notification.ErrInvalidTarget) {
		t.Errorf("Notify(empty target) = %v, want ErrInvalidTarget", err)
	}
	if called {
		t.Error("send called for invalid notification")
	}
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts notification.Options
	}{
		{"empty project", notification.Options{FCM: notification.FCMOptions{ServiceAccount: "svc.json"}}},
		{"empty service account", notification.Options{FCM: notification.FCMOptions{ProjectID: "p"}}},
		{"negative timeout", notification.Options{Timeout: -time.Second, FCM: notification.FCMOptions{ProjectID: "p", ServiceAccount: "svc.json"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(c.opts); !errors.Is(err, notification.ErrInvalidOptions) {
				t.Errorf("New() = %v, want ErrInvalidOptions", err)
			}
		})
	}
}

func TestNew_unreadableServiceAccount(t *testing.T) {
	t.Parallel()
	missing := filepath.Join(t.TempDir(), "missing.json")
	opts := notification.Options{FCM: notification.FCMOptions{ProjectID: "p", ServiceAccount: missing}}
	if _, err := New(opts); err == nil {
		t.Error("New(missing file) = nil, want load error")
	}
}

func TestValidateServiceAccountPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	validMissing := filepath.Join(dir, "svc.json")
	cases := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty", "", true},
		{"dot-dot element", "../evil.json", true},
		{"unclean", "a/../b.json", true},
		{"dot element unclean", "a/./b.json", true},
		{"wrong extension", filepath.Join(dir, "svc.key"), true},
		{"directory", dir, true},
		{"valid missing file", validMissing, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			err := validateServiceAccountPath(c.path)
			if (err != nil) != c.wantErr {
				t.Errorf("validateServiceAccountPath(%q) = %v, wantErr=%v", c.path, err, c.wantErr)
			}
		})
	}
}

func TestValidateServiceAccountPath_realFile(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "svc.json")
	if err := os.WriteFile(p, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write temp key = %v", err)
	}
	if err := validateServiceAccountPath(p); err != nil {
		t.Errorf("validateServiceAccountPath(real .json) = %v, want nil", err)
	}
}

func TestClose_idempotentNil(t *testing.T) {
	t.Parallel()
	var n notification.Notifier = &notifier{}
	if err := n.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if err := n.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}
