package twilio

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	twilioclient "github.com/twilio/twilio-go/client"

	"github.com/zenta-dev/zever/notification"
)

const (
	testAccountSID = "ACtestSid1234567890abcdef"
	testAuthToken  = "authTokenSecret123456"
	testFromNumber = "+14155552671"
	testToNumber   = "+14155552672"
)

// rewriteTransport redirects every request at the httptest server while
// preserving path and query. It wraps the notifier's own transport so the
// 1 MiB response got stays in the round-trip path.
type rewriteTransport struct {
	base string
	rt   http.RoundTripper
}

func (t *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, err := url.Parse(t.base)
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = target.Scheme
	clone.URL.Host = target.Host
	return t.rt.RoundTrip(clone)
}

type capturedRequest struct {
	to, from, body     string
	username, password string
	hasAuth            bool
	path               string
}

func validOptions() notification.Options {
	return notification.Options{Twilio: notification.TwilioOptions{
		AccountSID: testAccountSID,
		AuthToken:  testAuthToken,
		FromNumber: testFromNumber,
	}}
}

func validSMS() *notification.Notification {
	return &notification.Notification{
		Target:  testToNumber,
		Channel: notification.ChannelSMS,
		Body:    "hello",
	}
}

// newTestNotifier builds a notifier whose HTTP traffic is redirected at srv,
// recording the last request form fields, basic-auth, and path.
func newTestNotifier(t *testing.T, srv *httptest.Server) notification.Notifier {
	t.Helper()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	tn, ok := n.(*twilioNotifier)
	if !ok {
		t.Fatalf("New returned %T, want *twilioNotifier", n)
	}
	bc, ok := tn.client.Client.(*twilioclient.Client)
	if !ok || bc.HTTPClient == nil {
		t.Fatalf("notifier client is %T, want *twilioclient.Client with HTTPClient", tn.client.Client)
	}
	prev := bc.HTTPClient.Transport
	if prev == nil {
		prev = http.DefaultTransport
	}
	bc.HTTPClient.Transport = &rewriteTransport{base: srv.URL, rt: prev}
	return n
}

// stubTwilio serves canned Twilio Messages responses and records requests.
func stubTwilio(t *testing.T, got *capturedRequest, status int, respBody string, delay time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if delay > 0 {
			select {
			case <-time.After(delay):
			case <-r.Context().Done():
				return
			}
		}
		got.path = r.URL.Path
		u, p, ok := r.BasicAuth()
		got.username, got.password, got.hasAuth = u, p, ok
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm err = %v", err)
		}
		got.to = r.PostForm.Get("To")
		got.from = r.PostForm.Get("From")
		got.body = r.PostForm.Get("Body")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, respBody)
	}))
}

func TestNew_invalidOptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		opts notification.Options
	}{
		{"negative timeout", notification.Options{Timeout: -time.Second, Twilio: validOptions().Twilio}},
		{"empty AccountSID", notification.Options{Twilio: notification.TwilioOptions{AuthToken: testAuthToken, FromNumber: testFromNumber}}},
		{"empty AuthToken", notification.Options{Twilio: notification.TwilioOptions{AccountSID: testAccountSID, FromNumber: testFromNumber}}},
		{"empty FromNumber", notification.Options{Twilio: notification.TwilioOptions{AccountSID: testAccountSID, AuthToken: testAuthToken}}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if _, err := New(c.opts); !errors.Is(err, notification.ErrInvalidOptions) {
				t.Fatalf("New err = %v, want ErrInvalidOptions", err)
			}
			var ioe notification.InvalidOptionsError
			var ioePtr *notification.InvalidOptionsError
			if _, err := New(c.opts); !errors.As(err, &ioe) && !errors.As(err, &ioePtr) {
				t.Fatalf("New err %v is not InvalidOptionsError", err)
			}
		})
	}
}

func TestNotify_sendsFormWithBasicAuth(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123","status":"queued"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	if err := n.Notify(t.Context(), validSMS()); err != nil {
		t.Fatalf("Notify err = %v", err)
	}
	if got.to != testToNumber {
		t.Errorf("To = %q want %q", got.to, testToNumber)
	}
	if got.from != testFromNumber {
		t.Errorf("From = %q want %q", got.from, testFromNumber)
	}
	if got.body != "hello" {
		t.Errorf("Body = %q want %q", got.body, "hello")
	}
	if !got.hasAuth || got.username != testAccountSID || got.password != testAuthToken {
		t.Errorf("basic-auth = %q/%q present=%v, want %q/%q", got.username, got.password, got.hasAuth, testAccountSID, testAuthToken)
	}
	if !strings.Contains(got.path, "/2010-04-01/Accounts/"+testAccountSID+"/Messages.json") {
		t.Errorf("path = %q, want Messages endpoint for account", got.path)
	}
}

func TestNotify_nilNotification(t *testing.T) {
	t.Parallel()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	defer n.Close()
	if err := n.Notify(t.Context(), nil); !errors.Is(err, notification.ErrNilNotification) {
		t.Fatalf("Notify(nil) err = %v, want ErrNilNotification", err)
	}
}

func TestNotify_invalidTarget(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	bad := &notification.Notification{Target: "not-a-phone", Channel: notification.ChannelSMS, Body: "hi"}
	if err := n.Notify(t.Context(), bad); !errors.Is(err, notification.ErrInvalidTarget) {
		t.Fatalf("Notify(bad E.164) err = %v, want ErrInvalidTarget", err)
	}
}

func TestNotify_wrongChannel(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	push := &notification.Notification{Target: "device-token-abc", Channel: notification.ChannelPush, Body: "hi"}
	if err := n.Notify(t.Context(), push); !errors.Is(err, notification.ErrChannelNotSupported) {
		t.Fatalf("Notify(push) err = %v, want ErrChannelNotSupported", err)
	}
}

func TestNotify_serverError(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusBadRequest, `{"code":21211,"message":"invalid to","status":400,"more_info":"https://www.twilio.com/docs/errors/21211"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	err := n.Notify(t.Context(), validSMS())
	if err == nil {
		t.Fatal("Notify err = nil, want 400-mapped error")
	}
	if !strings.Contains(err.Error(), "twilio: send failed") {
		t.Errorf("Notify err %q missing %q", err.Error(), "twilio: send failed")
	}
}

func TestNotify_contextCanceled(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 0)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := n.Notify(ctx, validSMS()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Notify err = %v, want context.Canceled", err)
	}
}

func TestNotify_contextTimeout(t *testing.T) {
	t.Parallel()
	got := &capturedRequest{}
	srv := stubTwilio(t, got, http.StatusCreated, `{"sid":"SM123"}`, 500*time.Millisecond)
	defer srv.Close()
	n := newTestNotifier(t, srv)
	defer n.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	err := n.Notify(ctx, validSMS())
	if err == nil {
		t.Fatal("Notify err = nil, want timeout error")
	}
	if !strings.Contains(err.Error(), "twilio:") {
		t.Errorf("Notify err %q missing %q prefix", err.Error(), "twilio:")
	}
}

func TestClose_idempotent(t *testing.T) {
	t.Parallel()
	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("second Close err = %v, want nil", err)
	}
}
