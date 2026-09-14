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

	twilio "github.com/twilio/twilio-go"
	twilioclient "github.com/twilio/twilio-go/client"

	"github.com/zenta-dev/zever/notification"
)

// stubRoundTripper returns canned responses without network.
type stubRoundTripper struct {
	resp *http.Response
	err  error
}

func (s stubRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return s.resp, s.err
}

// fakeBaseClient is a non-*twilioclient.Client BaseClient for fallback tests.
type fakeBaseClient struct{ sid string }

func (f *fakeBaseClient) AccountSid() string       { return f.sid }
func (f *fakeBaseClient) SetTimeout(time.Duration) {}
func (f *fakeBaseClient) SendRequest(string, string, url.Values, map[string]interface{}, ...byte) (*http.Response, error) {
	return nil, errors.New("fakeBaseClient: no transport")
}
func (f *fakeBaseClient) SetOauth(twilioclient.OAuth) {}
func (f *fakeBaseClient) OAuth() twilioclient.OAuth   { return nil }

var _ twilioclient.BaseClient = (*fakeBaseClient)(nil)

// flipErrCtx simulates a deadline expiring between Notify's pre-check and
// apiForCall: first Err() is nil, later calls report DeadlineExceeded while
// Deadline() stays in the past. Done/Value mimic context.Background.
type flipErrCtx struct {
	calls int
}

func (c *flipErrCtx) Deadline() (time.Time, bool) {
	return time.Now().Add(-time.Second), true
}

func (c *flipErrCtx) Done() <-chan struct{} { return nil }

func (c *flipErrCtx) Err() error {
	c.calls++
	if c.calls == 1 {
		return nil
	}
	return context.DeadlineExceeded
}

func (c *flipErrCtx) Value(any) any { return nil }

// closeTracker records Close and remaining readable bytes.
type closeTracker struct {
	r      *strings.Reader
	closed bool
}

func (c *closeTracker) Read(p []byte) (int, error) { return c.r.Read(p) }
func (c *closeTracker) Close() error {
	c.closed = true
	return nil
}

func mustNotifier(t *testing.T, opts notification.Options) *twilioNotifier {
	t.Helper()
	n, err := New(opts)
	if err != nil {
		t.Fatalf("New err = %v", err)
	}
	tn, ok := n.(*twilioNotifier)
	if !ok {
		t.Fatalf("New returned %T, want *twilioNotifier", n)
	}
	return tn
}

func TestLimitedTransport_nilBaseUsesDefault(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest err = %v", err)
	}
	resp, err := (&limitedTransport{}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip err = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll err = %v", err)
	}
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestLimitedTransport_roundTripError(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // refused connection below

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("NewRequest err = %v", err)
	}
	resp, err := (&limitedTransport{base: http.DefaultTransport}).RoundTrip(req)
	if err == nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		t.Fatal("RoundTrip err = nil, want connection-refused error")
	}
}

func TestLimitedTransport_passthrough(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com/", nil)

	// Synthetic stub responses carry no live body; nothing to close.
	if resp, err := (stubRoundTripper{}).roundTrip(req); resp != nil || err != nil { //nolint:bodyclose
		t.Fatalf("(nil,nil) = (%v, %v), want (nil, nil)", resp, err)
	}

	bare := &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}
	resp, err := (stubRoundTripper{resp: bare}).roundTrip(req) //nolint:bodyclose
	if err != nil || resp != bare || resp.Body != nil {
		t.Fatalf("nil-body = (%v, %v), want bare resp passthrough", resp, err)
	}

	withBody := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("x")), Header: make(http.Header)}
	defer func() { _ = withBody.Body.Close() }()
	wantErr := errors.New("boom")
	gotResp, gotErr := (stubRoundTripper{resp: withBody, err: wantErr}).roundTrip(req) //nolint:bodyclose
	if !errors.Is(gotErr, wantErr) || gotResp != withBody {
		t.Fatalf("err passthrough = (%v, %v), want originals", gotResp, gotErr)
	}
}

func (s stubRoundTripper) roundTrip(req *http.Request) (*http.Response, error) {
	return (&limitedTransport{base: s}).RoundTrip(req)
}

func TestLimitedTransport_capsLargeBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(make([]byte, 2*maxResponseBytes))
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest err = %v", err)
	}
	resp, err := (&limitedTransport{base: http.DefaultTransport}).RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip err = %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll err = %v", err)
	}
	if len(body) != maxResponseBytes {
		t.Errorf("capped len = %d, want %d", len(body), maxResponseBytes)
	}
}

func TestLimitedReadCloser_direct(t *testing.T) {
	t.Parallel()
	ct := &closeTracker{r: strings.NewReader("hello world")}
	lrc := &limitedReadCloser{reader: io.LimitReader(ct, 5), orig: ct}

	buf := make([]byte, 2)
	if n, err := lrc.Read(buf); err != nil || n != 2 || string(buf) != "he" {
		t.Fatalf("Read = (%d, %v, %q), want (2, nil, he)", n, err, buf)
	}
	rest, err := io.ReadAll(lrc)
	if err != nil || string(rest) != "llo" {
		t.Fatalf("ReadAll = (%q, %v), want (llo, nil)", rest, err)
	}
	if err := lrc.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if !ct.closed {
		t.Error("Close did not close orig")
	}
	if ct.r.Len() != 0 {
		t.Errorf("Close left %d bytes undrained", ct.r.Len())
	}
}

func TestNew_fieldValues(t *testing.T) {
	t.Parallel()
	tn := mustNotifier(t, validOptions())
	if tn.timeout != notification.DefaultTimeout {
		t.Errorf("timeout = %v, want %v", tn.timeout, notification.DefaultTimeout)
	}
	if tn.fromNumber != testFromNumber {
		t.Errorf("fromNumber = %q, want %q", tn.fromNumber, testFromNumber)
	}
	bc, ok := tn.client.Client.(*twilioclient.Client)
	if !ok {
		t.Fatalf("client is %T, want *twilioclient.Client", tn.client.Client)
	}
	if bc.Credentials == nil {
		t.Fatal("Credentials = nil, want non-nil when creds set")
	}
	if bc.Username != testAccountSID || bc.Password != testAuthToken {
		t.Errorf("credentials = %q/***, want SID-auth pair", bc.Username)
	}
	if bc.HTTPClient == nil || bc.HTTPClient.Timeout != notification.DefaultTimeout {
		t.Errorf("HTTPClient.Timeout = %v, want %v", bc.HTTPClient, notification.DefaultTimeout)
	}

	opts := validOptions()
	opts.Timeout = 5 * time.Second
	tn2 := mustNotifier(t, opts)
	if tn2.timeout != 5*time.Second {
		t.Errorf("explicit timeout = %v, want 5s", tn2.timeout)
	}
	bc2, ok := tn2.client.Client.(*twilioclient.Client)
	if !ok || bc2.HTTPClient == nil || bc2.HTTPClient.Timeout != 5*time.Second {
		t.Errorf("explicit HTTPClient.Timeout not 5s: %v", bc2.HTTPClient)
	}
}

func TestNotify_invalidNotificationShape(t *testing.T) {
	t.Parallel()
	n := mustNotifier(t, validOptions())
	defer n.Close()

	bad := validSMS()
	bad.Title = "push-only title"
	if err := n.Notify(context.Background(), bad); !errors.Is(err, notification.ErrInvalidNotification) {
		t.Fatalf("Notify(title on sms) err = %v, want ErrInvalidNotification", err)
	}
}

func TestNotify_apiForCallExpired(t *testing.T) {
	t.Parallel()
	n := mustNotifier(t, validOptions())
	defer n.Close()

	err := n.Notify(&flipErrCtx{}, validSMS())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Notify err = %v, want DeadlineExceeded", err)
	}
	if !strings.Contains(err.Error(), "twilio:") {
		t.Errorf("Notify err %q missing %q prefix", err.Error(), "twilio:")
	}
}

func TestApiForCall_matrix(t *testing.T) {
	t.Parallel()
	tn := mustNotifier(t, validOptions())

	t.Run("unclamped shares Api", func(t *testing.T) {
		t.Parallel()
		api, err := tn.apiForCall(context.Background())
		if err != nil {
			t.Fatalf("apiForCall err = %v", err)
		}
		if api != tn.client.Api {
			t.Error("unclamped apiForCall did not return shared Api")
		}
	})

	t.Run("clamped clones with remaining timeout", func(t *testing.T) {
		t.Parallel()
		bc, ok := tn.client.Client.(*twilioclient.Client)
		if !ok || bc.HTTPClient == nil {
			t.Fatalf("client is %T, want usable *twilioclient.Client", tn.client.Client)
		}
		origTimeout := bc.HTTPClient.Timeout
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer cancel()
		api, err := tn.apiForCall(ctx)
		if err != nil {
			t.Fatalf("apiForCall err = %v", err)
		}
		if api == tn.client.Api {
			t.Fatal("clamped apiForCall returned shared Api, want clone")
		}
		cloned, ok := api.RequestHandler().Client.(*twilioclient.Client)
		if !ok || cloned.HTTPClient == nil {
			t.Fatalf("clone client is %T, want *twilioclient.Client with HTTPClient", api.RequestHandler().Client)
		}
		if cloned.HTTPClient.Timeout <= 0 || cloned.HTTPClient.Timeout > time.Second {
			t.Errorf("clone Timeout = %v, want (0, 1s]", cloned.HTTPClient.Timeout)
		}
		if bc.HTTPClient.Timeout != origTimeout {
			t.Errorf("shared HTTPClient.Timeout = %v, want unchanged %v", bc.HTTPClient.Timeout, origTimeout)
		}
	})

	t.Run("expired returns ctx err", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		api, err := tn.apiForCall(ctx)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("apiForCall err = %v, want DeadlineExceeded", err)
		}
		if api != nil {
			t.Errorf("apiForCall api = %v, want nil on expired", api)
		}
	})

	t.Run("non-twilio client shares Api", func(t *testing.T) {
		t.Parallel()
		client := twilio.NewRestClientWithParams(twilio.ClientParams{Client: &fakeBaseClient{sid: testAccountSID}})
		fb := &twilioNotifier{client: client, fromNumber: testFromNumber, timeout: notification.DefaultTimeout}
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer cancel()
		api, err := fb.apiForCall(ctx)
		if err != nil {
			t.Fatalf("apiForCall err = %v", err)
		}
		if api != fb.client.Api {
			t.Error("non-twilio client did not fall back to shared Api")
		}
	})

	t.Run("nil HTTPClient shares Api", func(t *testing.T) {
		t.Parallel()
		base := &twilioclient.Client{Credentials: twilioclient.NewCredentials(testAccountSID, testAuthToken)}
		client := twilio.NewRestClientWithParams(twilio.ClientParams{Client: base})
		nb := &twilioNotifier{client: client, fromNumber: testFromNumber, timeout: notification.DefaultTimeout}
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
		defer cancel()
		api, err := nb.apiForCall(ctx)
		if err != nil {
			t.Fatalf("apiForCall err = %v", err)
		}
		if api != nb.client.Api {
			t.Error("nil-HTTPClient client did not fall back to shared Api")
		}
	})
}

func TestDeadlineTimeout_arms(t *testing.T) {
	t.Parallel()
	tn := &twilioNotifier{timeout: notification.DefaultTimeout}

	if got, clamped := tn.deadlineTimeout(context.Background()); clamped || got != notification.DefaultTimeout {
		t.Errorf("no deadline = (%v, %v), want (%v, false)", got, clamped, notification.DefaultTimeout)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if got, clamped := tn.deadlineTimeout(ctx); !clamped || got <= 0 || got > time.Second {
		t.Errorf("nearer deadline = (%v, %v), want (<=1s, true)", got, clamped)
	}

	short := &twilioNotifier{timeout: 50 * time.Millisecond}
	far, cancelFar := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFar()
	if got, clamped := short.deadlineTimeout(far); clamped || got != 50*time.Millisecond {
		t.Errorf("farther deadline = (%v, %v), want (50ms, false)", got, clamped)
	}
}
