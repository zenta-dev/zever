package log

import (
	"errors"
	"io"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/notification"
)

type failWriter struct{}

func (failWriter) Write(_ []byte) (int, error) { return 0, errors.New("write failed") }

func TestCover_NewWithWriter_NilWriterDefaultsToStdout(t *testing.T) {
	old := os.Stdout
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("os.Pipe err = %v", pipeErr)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()
	n, err := NewWithWriter(notification.Options{}, nil)
	if err != nil {
		w.Close()
		t.Fatalf("NewWithWriter(nil) err = %v", err)
	}
	defer n.Close()
	sent := &notification.Notification{Target: "tok", Channel: notification.ChannelPush, Body: "hi"}
	if notifyErr := n.Notify(t.Context(), sent); notifyErr != nil {
		w.Close()
		t.Fatalf("Notify err = %v", notifyErr)
	}
	if closeErr := w.Close(); closeErr != nil {
		t.Fatalf("pipe close err = %v", closeErr)
	}
	out, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("ReadAll err = %v", readErr)
	}
	if !strings.Contains(string(out), `"target":"tok"`) {
		t.Errorf("stdout output missing target echo: %s", out)
	}
}

func TestCover_Notify_ClosedAfterLock(t *testing.T) {
	t.Parallel()
	var discard discardWriter
	n, err := NewWithWriter(notification.Options{}, discard)
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()
	c, ok := n.(*checker)
	if !ok {
		t.Fatalf("notifier type = %T, want *checker", n)
	}
	c.mu.Lock()
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		done <- c.Notify(t.Context(), &notification.Notification{
			Target:  "tok",
			Channel: notification.ChannelPush,
			Body:    "hi",
		})
	}()
	// Yield so the goroutine passes the pre-lock closed check and parks on
	// c.mu, so the post-lock closed check is the one that fires (either
	// branch returns ErrClosed; no fixed sleep).
	<-started
	for range 100 {
		runtime.Gosched()
	}
	c.closed.Store(true)
	c.mu.Unlock()
	select {
	case err := <-done:
		if !errors.Is(err, notification.ErrClosed) {
			t.Fatalf("Notify err = %v, want ErrClosed", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Notify did not return after unlock")
	}
}

func TestCover_Notify_EncodeError(t *testing.T) {
	t.Parallel()
	n, err := NewWithWriter(notification.Options{}, failWriter{})
	if err != nil {
		t.Fatalf("NewWithWriter err = %v", err)
	}
	defer n.Close()
	err = n.Notify(t.Context(), &notification.Notification{
		Target:  "tok",
		Channel: notification.ChannelPush,
		Body:    "hi",
	})
	if err == nil {
		t.Fatal("Notify err = nil, want encode error")
	}
	if !strings.Contains(err.Error(), "log:") {
		t.Errorf("Notify err %q missing %q prefix", err.Error(), "log:")
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
