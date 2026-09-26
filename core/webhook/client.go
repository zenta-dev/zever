package webhook

import (
	"net/http"
	"time"

	"github.com/zenta-dev/zever/shared/httpclient"
)

// NewSafeClient returns an *http.Client that enforces TLS 1.2 minimum,
// refuses private-address dials unless allowPrivate is true, never follows
// redirects (CheckRedirect returns http.ErrUseLastResponse so callers see the
// 3xx response), and applies timeout to the whole request.
func NewSafeClient(timeout time.Duration, allowPrivate bool) *http.Client {
	return httpclient.NewClient(timeout, httpclient.WithSafeDial(allowPrivate), httpclient.WithNoRedirect())
}
