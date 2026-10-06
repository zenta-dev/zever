package twilio

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zenta-dev/zever/core/notification"
)

func TestNotify_emptyTarget(t *testing.T) {
	t.Parallel()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer n.Close()
	in := &notification.Notification{Target: "", Channel: notification.ChannelSMS, Body: "hi"}
	if err := n.Notify(t.Context(), in); !errors.Is(err, notification.ErrInvalidTarget) {
		t.Errorf("Notify(empty target) = %v, want ErrInvalidTarget", err)
	}
}

func TestNotify_veryLongBody(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	in := &notification.Notification{
		Target:  testToNumber,
		Channel: notification.ChannelSMS,
		Body:    strings.Repeat("a", 1<<20),
	}
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.body != in.Body {
		t.Errorf("body length = %d, want %d", len(got.body), len(in.Body))
	}
}

func TestNotify_priorityAndTTLIgnoredForSMS(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	in := validSMS()
	in.Priority = notification.PriorityHigh
	in.TTL = 1 << 30
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil (SMS accepts priority/TTL and ignores them)", err)
	}
	if got.body != "hello" {
		t.Errorf("Body = %q, want hello", got.body)
	}
}

// TestNotify_concurrentSends uses a recording-free stub so parallel handler
// goroutines never race on shared capture state.
func TestNotify_concurrentSends(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sid":"SM123"}`))
	}))
	defer srv.Close()

	n := newTestNotifier(t, srv)
	defer n.Close()

	const workers = 8
	const perWorker = 10
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if err := n.Notify(t.Context(), validSMS()); err != nil {
					errs[w] = err
					return
				}
			}
		}()
	}
	wg.Wait()
	for w, err := range errs {
		if err != nil {
			t.Errorf("worker %d Notify() = %v, want nil", w, err)
		}
	}
}

func TestNotify_afterClose_stillSends(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	if err := n.Close(); err != nil {
		t.Fatalf("Close() = %v, want nil", err)
	}
	if err := n.Notify(t.Context(), validSMS()); err != nil {
		t.Errorf("Notify after Close = %v, want nil (Close releases nothing)", err)
	}
}

func TestNotify_smsWithTitle_rejects(t *testing.T) {
	t.Parallel()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer n.Close()
	in := validSMS()
	in.Title = "push-only"
	if err := n.Notify(t.Context(), in); !errors.Is(err, notification.ErrInvalidNotification) {
		t.Errorf("Notify(sms+title) = %v, want ErrInvalidNotification", err)
	}
}

func TestNotify_smsWithData_rejects(t *testing.T) {
	t.Parallel()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer n.Close()
	in := validSMS()
	in.Data = map[string]string{"k": "v"}
	if err := n.Notify(t.Context(), in); !errors.Is(err, notification.ErrInvalidNotification) {
		t.Errorf("Notify(sms+data) = %v, want ErrInvalidNotification", err)
	}
}

func TestNotify_negativeTTL_rejects(t *testing.T) {
	t.Parallel()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer n.Close()
	in := validSMS()
	in.TTL = -time.Second
	if err := n.Notify(t.Context(), in); !errors.Is(err, notification.ErrInvalidNotification) {
		t.Errorf("Notify(negative ttl) = %v, want ErrInvalidNotification", err)
	}
}

func TestNotify_unknownPriority_rejects(t *testing.T) {
	t.Parallel()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New() = %v", err)
	}
	defer n.Close()
	in := validSMS()
	in.Priority = notification.Priority("urgent")
	if err := n.Notify(t.Context(), in); !errors.Is(err, notification.ErrInvalidNotification) {
		t.Errorf("Notify(unknown priority) = %v, want ErrInvalidNotification", err)
	}
}

func TestNotify_emptyBody_sends(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()
	in := &notification.Notification{
		Target:  testToNumber,
		Channel: notification.ChannelSMS,
	}
	if err := n.Notify(t.Context(), in); err != nil {
		t.Fatalf("Notify() = %v, want nil", err)
	}
	if got.body != "" {
		t.Errorf("Body = %q, want empty", got.body)
	}
}
