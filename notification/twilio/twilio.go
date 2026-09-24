package twilio

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	twilio "github.com/twilio/twilio-go"
	twilioclient "github.com/twilio/twilio-go/client"
	openapi "github.com/twilio/twilio-go/rest/api/v2010"

	"github.com/zenta-dev/zever/internal/httpclient"
	"github.com/zenta-dev/zever/notification"
)

// maxResponseBytes caps the bytes read from a Twilio response body.
const maxResponseBytes = 1 << 20 // 1 MiB

// limitedTransport caps Twilio response bodies so a misbehaving server
// cannot force unbounded memory use.
type limitedTransport struct {
	base http.RoundTripper
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rt := t.base
	if rt == nil {
		rt = http.DefaultTransport
	}
	resp, err := rt.RoundTrip(req)
	if err != nil || resp == nil || resp.Body == nil {
		return resp, err
	}
	resp.Body = &limitedReadCloser{reader: io.LimitReader(resp.Body, maxResponseBytes), orig: resp.Body}
	return resp, nil
}

type limitedReadCloser struct {
	reader io.Reader
	orig   io.ReadCloser
}

func (l *limitedReadCloser) Read(p []byte) (int, error) { return l.reader.Read(p) }

func (l *limitedReadCloser) Close() error {
	_, _ = io.Copy(io.Discard, l.reader)
	_, _ = io.Copy(io.Discard, l.orig)
	return l.orig.Close()
}

type twilioNotifier struct {
	client     *twilio.RestClient
	fromNumber string
	timeout    time.Duration
}

var _ notification.Notifier = (*twilioNotifier)(nil)

// New validates opts and returns a Notifier sending SMS via Twilio.
// Zero Timeout resolves to notification.DefaultTimeout.
func New(opts notification.Options) (notification.Notifier, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("twilio: invalid options: %w", err)
	}
	tw := opts.Twilio
	if tw.AccountSID == "" {
		return nil, fmt.Errorf("twilio: %w", notification.InvalidOptionsError{Reason: "twilio account sid must be non-empty"})
	}
	if tw.AuthToken == "" {
		return nil, fmt.Errorf("twilio: %w", notification.InvalidOptionsError{Reason: "twilio auth token must be non-empty"})
	}
	if tw.FromNumber == "" {
		return nil, fmt.Errorf("twilio: %w", notification.InvalidOptionsError{Reason: "twilio from number must be non-empty"})
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = notification.DefaultTimeout
	}
	base := &twilioclient.Client{
		Credentials: twilioclient.NewCredentials(tw.AccountSID, tw.AuthToken),
		HTTPClient: &http.Client{
			Timeout:   timeout,
			Transport: &limitedTransport{base: httpclient.NewClient(timeout).Transport},
		},
	}
	base.SetAccountSid(tw.AccountSID)
	client := twilio.NewRestClientWithParams(twilio.ClientParams{Client: base})
	return &twilioNotifier{client: client, fromNumber: tw.FromNumber, timeout: timeout}, nil
}

// Notify sends n as an SMS from the configured FromNumber.
// Title and Data are rejected by core validation (push-only); Priority and
// TTL are accepted but ignored because SMS has no such fields.
//
// Deliberately not retried on transient failure (unlike notification/fcm's
// push Notify): this SDK's Messages API has no idempotency-key parameter,
// so a caller-side retry after an ambiguous failure (the request may have
// already reached Twilio and been billed/sent before the error surfaced)
// risks a duplicate paid SMS with no dedup mechanism available -- a
// materially worse failure mode than a duplicate push notification.
func (t *twilioNotifier) Notify(ctx context.Context, n *notification.Notification) error {
	if n == nil {
		return notification.ErrNilNotification
	}
	if err := n.Validate(); err != nil {
		return err
	}
	if n.Channel != notification.ChannelSMS {
		return fmt.Errorf("twilio: %w: channel %q", notification.ErrChannelNotSupported, string(n.Channel))
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("twilio: %w", err)
	}
	api, err := t.apiForCall(ctx)
	if err != nil {
		return fmt.Errorf("twilio: %w", err)
	}
	params := &openapi.CreateMessageParams{}
	params.SetTo(n.Target)
	params.SetFrom(t.fromNumber)
	params.SetBody(n.Body)
	if _, err := api.CreateMessage(params); err != nil {
		return fmt.Errorf("twilio: send failed: %w", err)
	}
	return nil
}

// Close shuts down the notifier. It is idempotent and always returns nil:
// sends use a shared client with no per-notifier resources to release.
func (t *twilioNotifier) Close() error { return nil }

// apiForCall returns the shared API handle, or a per-call handle whose HTTP
// timeout is clamped to the earlier ctx deadline. The Twilio SDK builds its
// requests on context.Background, so ctx cancellation cannot abort a send in
// flight; clamping bounds the wait instead.
func (t *twilioNotifier) apiForCall(ctx context.Context) (*openapi.ApiService, error) {
	timeout, clamped := t.deadlineTimeout(ctx)
	if clamped && timeout <= 0 {
		return nil, ctx.Err()
	}
	if !clamped {
		return t.client.Api, nil
	}
	bc, ok := t.client.Client.(*twilioclient.Client)
	if !ok || bc.HTTPClient == nil || t.client.RequestHandler == nil {
		return t.client.Api, nil
	}
	clone := *bc
	hc := *bc.HTTPClient
	hc.Timeout = timeout
	clone.HTTPClient = &hc
	handler := *t.client.RequestHandler
	handler.Client = &clone
	return openapi.NewApiService(&handler), nil
}

func (t *twilioNotifier) deadlineTimeout(ctx context.Context) (time.Duration, bool) {
	dl, ok := ctx.Deadline()
	if !ok {
		return t.timeout, false
	}
	if remaining := time.Until(dl); remaining < t.timeout {
		return remaining, true
	}
	return t.timeout, false
}
