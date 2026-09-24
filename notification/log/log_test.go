package log_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/notification"
	notificationlog "github.com/zenta-dev/zever/notification/log"
)

func validNotification() *notification.Notification {
	return &notification.Notification{
		Target:   "push-token-123",
		Channel:  notification.ChannelPush,
		Title:    "hello",
		Body:     "world",
		Data:     map[string]string{"k": "v"},
		Priority: notification.PriorityHigh,
		TTL:      90 * time.Second,
	}
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()
	if _, err := notificationlog.New(notification.Options{Timeout: -1}); !errors.Is(err, notification.ErrInvalidOptions) {
		t.Fatalf("New err = %v, want ErrInvalidOptions", err)
	}
	var buf bytes.Buffer
	if _, err := notificationlog.NewWithWriter(notification.Options{Timeout: -1}, &buf); !errors.Is(err, notification.ErrInvalidOptions) {
		t.Fatalf("NewWithWriter err = %v, want ErrInvalidOptions", err)
	}
}

func TestNotify_nilNotification(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()
	if err := n.Notify(t.Context(), nil); !errors.Is(err, notification.ErrNilNotification) {
		t.Fatalf("Notify(nil) err = %v, want ErrNilNotification", err)
	}
}

func TestNotify_invalidNotification(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()
	bad := &notification.Notification{
		Target:  "not-a-phone",
		Channel: notification.ChannelSMS,
		Body:    "hi",
	}
	if err := n.Notify(t.Context(), bad); !errors.Is(err, notification.ErrInvalidTarget) {
		t.Fatalf("Notify(bad E.164) err = %v, want ErrInvalidTarget", err)
	}
}

func TestNotify_contextCanceled(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := n.Notify(ctx, validNotification()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Notify err = %v, want context.Canceled", err)
	}
}

func TestNotify_emitsValidJSON(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()
	want := validNotification()
	if err := n.Notify(t.Context(), want); err != nil {
		t.Fatalf("Notify err = %v", err)
	}
	line := strings.TrimSpace(buf.String())
	var decoded struct {
		TS        time.Time         `json:"ts"`
		Channel   string            `json:"channel"`
		Target    string            `json:"target"`
		Title     string            `json:"title"`
		Body      string            `json:"body"`
		Data      map[string]string `json:"data"`
		Priority  string            `json:"priority"`
		TTLSecond float64           `json:"ttl_seconds"`
	}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, line)
	}
	if decoded.Channel != string(want.Channel) {
		t.Errorf("channel = %q, want %q", decoded.Channel, want.Channel)
	}
	if decoded.Target != want.Target {
		t.Errorf("target = %q, want %q (dev sink must echo target)", decoded.Target, want.Target)
	}
	if decoded.Title != want.Title || decoded.Body != want.Body {
		t.Errorf("title/body = %q/%q, want %q/%q", decoded.Title, decoded.Body, want.Title, want.Body)
	}
	if decoded.Data["k"] != "v" {
		t.Errorf("data = %v, want k=v", decoded.Data)
	}
	if decoded.Priority != string(want.Priority) {
		t.Errorf("priority = %q, want %q", decoded.Priority, want.Priority)
	}
	if decoded.TTLSecond != want.TTL.Seconds() {
		t.Errorf("ttl_seconds = %v, want %v", decoded.TTLSecond, want.TTL.Seconds())
	}
	if decoded.TS.IsZero() {
		t.Errorf("ts is zero, want non-zero timestamp")
	}
}

func TestClose_idempotentAndAfterClose(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("second Close err = %v, want nil", err)
	}
	if err := n.Notify(t.Context(), validNotification()); !errors.Is(err, notification.ErrClosed) {
		t.Fatalf("Notify after close err = %v, want ErrClosed", err)
	}
}

func TestConcurrentSends(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()
	const count = 20
	var wg sync.WaitGroup
	errs := make([]error, count)
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = n.Notify(t.Context(), validNotification())
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("send %d err = %v", i, err)
		}
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != count {
		t.Fatalf("lines = %d, want %d", len(lines), count)
	}
	for i, line := range lines {
		var v map[string]any
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			t.Fatalf("line %d not valid JSON: %v", i, err)
		}
	}
}
