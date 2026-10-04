package log_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	notificationlog "github.com/zenta-dev/zever/adapters/notification/log"
	"github.com/zenta-dev/zever/core/notification"
)

type errWriter struct{ err error }

func (w errWriter) Write([]byte) (int, error) { return 0, w.err }

func TestNotify_writerError(t *testing.T) {
	t.Parallel()
	boom := errors.New("disk full")
	n, err := notificationlog.NewWithWriter(notification.Options{}, errWriter{err: boom})
	if err != nil {
		t.Fatalf("NewWithWriter() = %v", err)
	}
	defer n.Close()
	err = n.Notify(t.Context(), validNotification())
	if !errors.Is(err, boom) {
		t.Errorf("Notify() = %v, want wrap of boom", err)
	}
}

func TestNotify_emptyTitleBody(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() = %v", err)
	}
	defer n.Close()
	in := &notification.Notification{
		Target:  "push-token-123",
		Channel: notification.ChannelPush,
		Body:    "only-body",
	}
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	line := strings.TrimSpace(buf.String())
	if !strings.Contains(line, `"title":""`) {
		t.Errorf("line %q missing empty title", line)
	}
	if strings.Contains(line, `"data"`) {
		t.Errorf("line %q should omit empty data (omitempty)", line)
	}
}

func TestNotify_veryLongBody(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() = %v", err)
	}
	defer n.Close()
	in := &notification.Notification{
		Target:  "push-token-123",
		Channel: notification.ChannelPush,
		Body:    strings.Repeat("x", 1<<20),
	}
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if buf.Len() < 1<<20 {
		t.Errorf("wrote %d bytes, want >= 1 MiB body", buf.Len())
	}
}

func TestNotify_emptyDataTableOmitted(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	n, err := notificationlog.NewWithWriter(notification.Options{}, &buf)
	if err != nil {
		t.Fatalf("NewWithWriter() = %v", err)
	}
	defer n.Close()
	in := &notification.Notification{
		Target:  "push-token-123",
		Channel: notification.ChannelPush,
		Body:    "b",
		Data:    map[string]string{},
	}
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if strings.Contains(buf.String(), "data") {
		t.Errorf("line %q should omit empty data", buf.String())
	}
}

func TestNewWithWriter_nilWriterDefaultsToStdout(t *testing.T) {
	t.Parallel()
	n, err := notificationlog.NewWithWriter(notification.Options{}, nil)
	if err != nil {
		t.Fatalf("NewWithWriter(nil) = %v, want nil", err)
	}
	if err := n.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
}
