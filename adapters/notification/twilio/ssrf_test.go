package twilio

import (
	"errors"
	"net/http"
	"testing"

	twilioclient "github.com/twilio/twilio-go/client"

	"github.com/zenta-dev/zever/shared/httpclient"
)

// TestNew_TransportIsSafeByDefault verifies the Twilio transport refuses
// private dials and never follows redirects: the API host is fixed and
// public, so no opt-out exists.
func TestNew_TransportIsSafeByDefault(t *testing.T) {
	t.Parallel()

	n, err := New(validOptions())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer func() { _ = n.Close() }()

	tn, ok := n.(*twilioNotifier)
	if !ok {
		t.Fatalf("New() = %T, want *twilioNotifier", n)
	}

	bc, ok := tn.client.Client.(*twilioclient.Client)
	if !ok || bc.HTTPClient == nil {
		t.Fatalf("notifier client is %T, want *twilioclient.Client with HTTPClient", tn.client.Client)
	}

	if bc.HTTPClient.CheckRedirect == nil {
		t.Fatal("CheckRedirect nil: redirects must never be followed")
	}

	lt, ok := bc.HTTPClient.Transport.(*limitedTransport)
	if !ok {
		t.Fatalf("transport = %T, want *limitedTransport", bc.HTTPClient.Transport)
	}

	tr, ok := lt.base.(*http.Transport)
	if !ok {
		t.Fatalf("base transport = %T, want *http.Transport", lt.base)
	}

	if tr.DialContext == nil {
		t.Fatal("DialContext nil: safe dial guard not installed")
	}

	if _, err := tr.DialContext(t.Context(), "tcp", "169.254.169.254:443"); !errors.Is(err, httpclient.ErrPrivateAddress) {
		t.Fatalf("DialContext() error = %v, want ErrPrivateAddress", err)
	}
}
