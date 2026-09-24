package posthog

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	posthog "github.com/posthog/posthog-go"

	"github.com/zenta-dev/zever/analytics"
)

// errFakeClient simulates SDK client construction failures.
var errFakeClient = errors.New("fake client boom")

// fakeClient records enqueued messages without touching the network.
type fakeClient struct {
	posthog.EnqueueClient

	mu         sync.Mutex
	messages   []posthog.Message
	enqueueErr error
	closeErr   error
	closes     int
}

// Enqueue records the message or returns the configured error.
func (f *fakeClient) Enqueue(msg posthog.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.enqueueErr != nil {
		return f.enqueueErr
	}

	f.messages = append(f.messages, msg)

	return nil
}

// Close records the call and returns the configured error.
func (f *fakeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.closes++

	return f.closeErr
}

// count returns the number of recorded messages.
func (f *fakeClient) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.messages)
}

// first returns the first recorded message.
func (f *fakeClient) first() posthog.Message {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.messages[0]
}

// stubAdapter returns an adapter wired to a fake client.
func stubAdapter(_ *testing.T) (*adapter, *fakeClient) {
	f := &fakeClient{}

	a := &adapter{
		client:             f,
		anonymousID:        "anonymous",
		groupType:          "organization",
		maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
	}

	return a, f
}

func TestTrack_ctxUserID_usesDistinctID(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Track(analytics.WithUserID(t.Context(), "u1"), "signed_up", nil); err != nil {
		t.Fatal(err)
	}

	c, ok := f.first().(posthog.Capture)
	if !ok {
		t.Fatalf("want Capture, got %T", f.first())
	}

	if c.DistinctId != "u1" {
		t.Fatalf("DistinctId: got %q, want u1", c.DistinctId)
	}

	if c.Event != "signed_up" {
		t.Fatalf("Event: got %q, want signed_up", c.Event)
	}
}

func TestTrack_anonymous_fallsBackToDefault(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Track(t.Context(), "signed_up", nil); err != nil {
		t.Fatal(err)
	}

	c, ok := f.first().(posthog.Capture)
	if !ok {
		t.Fatalf("want Capture, got %T", f.first())
	}

	if c.DistinctId != "anonymous" {
		t.Fatalf("DistinctId: got %q, want anonymous", c.DistinctId)
	}
}

func TestTrack_anonymous_fallsBackToCustomID(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.anonymousID = "guest"

	if err := a.Track(t.Context(), "signed_up", nil); err != nil {
		t.Fatal(err)
	}

	c, ok := f.first().(posthog.Capture)
	if !ok {
		t.Fatalf("want Capture, got %T", f.first())
	}

	if c.DistinctId != "guest" {
		t.Fatalf("DistinctId: got %q, want guest", c.DistinctId)
	}
}

func TestTrack_missingIdentity_returnsSentinel(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.anonymousID = ""

	if err := a.Track(t.Context(), "signed_up", nil); !errors.Is(err, analytics.ErrMissingIdentity) {
		t.Fatalf("want ErrMissingIdentity, got %v", err)
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestTrack_enqueueError_wrapsCause(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	fakeErr := errors.New("boom")
	f.enqueueErr = fakeErr

	if err := a.Track(t.Context(), "signed_up", nil); !errors.Is(err, fakeErr) {
		t.Fatalf("want wrapped boom, got %v", err)
	}
}

func TestIdentify_traits_notMutatingCallerMap(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	traits := map[string]any{"plan": "pro"}
	if err := a.Identify(t.Context(), "user-1", traits); err != nil {
		t.Fatal(err)
	}

	if _, ok := traits["$anon_distinct_id"]; ok {
		t.Fatalf("Identify mutated caller's traits map: %v", traits)
	}

	e, ok := f.first().(posthog.Identify)
	if !ok {
		t.Fatalf("want Identify, got %T", f.first())
	}

	if e.Properties["$anon_distinct_id"] != "anonymous" {
		t.Fatalf("$anon_distinct_id = %v, want anonymous", e.Properties["$anon_distinct_id"])
	}

	if e.Properties["plan"] != "pro" {
		t.Fatalf("plan = %v, want pro", e.Properties["plan"])
	}
}

func TestIdentify_explicitID_sendsIdentifyMessage(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Identify(t.Context(), "u1", map[string]any{"email": "a@b.c"}); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.Identify)
	if !ok {
		t.Fatalf("want Identify, got %T", f.first())
	}

	if msg.DistinctId != "u1" {
		t.Fatalf("DistinctId: got %q, want u1", msg.DistinctId)
	}

	if got := msg.Properties["email"]; got != "a@b.c" {
		t.Fatalf("email: got %v, want a@b.c", got)
	}
}

func TestIdentify_ctxUserID_usesContextIdentity(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	ctx := analytics.WithUserID(t.Context(), "u1")
	if err := a.Identify(ctx, "", map[string]any{"email": "a@b.c"}); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.Identify)
	if !ok {
		t.Fatalf("want Identify, got %T", f.first())
	}

	if msg.DistinctId != "u1" {
		t.Fatalf("DistinctId: got %q, want u1", msg.DistinctId)
	}
}

func TestIdentify_anonymous_fallsBackToDefault(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Identify(t.Context(), "", map[string]any{"email": "a@b.c"}); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.Identify)
	if !ok {
		t.Fatalf("want Identify, got %T", f.first())
	}

	if msg.DistinctId != "anonymous" {
		t.Fatalf("DistinctId: got %q, want anonymous", msg.DistinctId)
	}

	if _, ok := msg.Properties["$anon_distinct_id"]; ok {
		t.Fatalf("$anon_distinct_id: got %v, want absent", msg.Properties["$anon_distinct_id"])
	}
}

func TestIdentify_missingIdentity_returnsSentinel(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.anonymousID = ""

	if err := a.Identify(t.Context(), "", nil); !errors.Is(err, analytics.ErrMissingIdentity) {
		t.Fatalf("want ErrMissingIdentity, got %v", err)
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestIdentify_anonymousDisabled_omitsBridgeProperty(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.anonymousID = ""

	if err := a.Identify(t.Context(), "u1", nil); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.Identify)
	if !ok {
		t.Fatalf("want Identify, got %T", f.first())
	}

	if _, ok := msg.Properties["$anon_distinct_id"]; ok {
		t.Fatalf("$anon_distinct_id: got %v, want absent", msg.Properties["$anon_distinct_id"])
	}
}

func TestIdentify_enqueueError_wrapsCause(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	fakeErr := errors.New("boom")
	f.enqueueErr = fakeErr

	if err := a.Identify(t.Context(), "u1", nil); !errors.Is(err, fakeErr) {
		t.Fatalf("want wrapped boom, got %v", err)
	}
}

func TestGroup_sendsGroupIdentifyMessage(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Group(t.Context(), "u1", "org-1", map[string]any{"name": "acme"}); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.GroupIdentify)
	if !ok {
		t.Fatalf("want GroupIdentify, got %T", f.first())
	}

	if msg.Type != "organization" {
		t.Fatalf("Type: got %q, want organization", msg.Type)
	}

	if msg.Key != "org-1" {
		t.Fatalf("Key: got %q, want org-1", msg.Key)
	}

	if got := msg.Properties["name"]; got != "acme" {
		t.Fatalf("name: got %v, want acme", got)
	}

	if err := msg.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestGroup_anonymous_fallsBackToDefault(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Group(t.Context(), "", "org-1", nil); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.GroupIdentify)
	if !ok {
		t.Fatalf("want GroupIdentify, got %T", f.first())
	}

	if msg.DistinctId != "anonymous" {
		t.Fatalf("DistinctId: got %q, want anonymous", msg.DistinctId)
	}
}

func TestGroup_ctxUserID_usesContextIdentity(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	ctx := analytics.WithUserID(t.Context(), "u1")
	if err := a.Group(ctx, "", "org-1", nil); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.GroupIdentify)
	if !ok {
		t.Fatalf("want GroupIdentify, got %T", f.first())
	}

	if msg.DistinctId != "u1" {
		t.Fatalf("DistinctId: got %q, want u1", msg.DistinctId)
	}
}

func TestGroup_paramID_takesPrecedenceOverContext(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	ctx := analytics.WithUserID(t.Context(), "ctx-user")
	if err := a.Group(ctx, "param-user", "org-1", nil); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.GroupIdentify)
	if !ok {
		t.Fatalf("want GroupIdentify, got %T", f.first())
	}

	if msg.DistinctId != "param-user" {
		t.Fatalf("DistinctId: got %q, want param-user", msg.DistinctId)
	}
}

func TestGroup_missingIdentity_returnsSentinel(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.anonymousID = ""

	if err := a.Group(t.Context(), "", "org-1", nil); !errors.Is(err, analytics.ErrMissingIdentity) {
		t.Fatalf("want ErrMissingIdentity, got %v", err)
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestGroup_missingGroupID_returnsSentinel(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Group(t.Context(), "u1", "", nil); !errors.Is(err, analytics.ErrMissingGroupID) {
		t.Fatalf("want ErrMissingGroupID, got %v", err)
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestGroup_customType_appliesToMessage(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.groupType = "workspace"

	if err := a.Group(t.Context(), "u1", "ws-1", nil); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.GroupIdentify)
	if !ok {
		t.Fatalf("want GroupIdentify, got %T", f.first())
	}

	if msg.Type != "workspace" {
		t.Fatalf("Type: got %q, want workspace", msg.Type)
	}
}

func TestGroup_enqueueError_wrapsCause(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	fakeErr := errors.New("boom")
	f.enqueueErr = fakeErr

	if err := a.Group(t.Context(), "u1", "org-1", nil); !errors.Is(err, fakeErr) {
		t.Fatalf("want wrapped boom, got %v", err)
	}
}

func TestOpen_missingKey_returnsErrMissingAPIKey(t *testing.T) {
	t.Parallel()

	if _, err := New(analytics.Options{}); !errors.Is(err, ErrMissingAPIKey) {
		t.Fatalf("want ErrMissingAPIKey, got %v", err)
	}
}

func TestOpen_invalidOptions_returnsErrInvalidOptions(t *testing.T) {
	t.Parallel()

	opts := analytics.Options{APIKey: "test-key", MaxPropertiesBytes: -1}
	if _, err := New(opts); !errors.Is(err, analytics.ErrInvalidOptions) {
		t.Fatalf("want ErrInvalidOptions, got %v", err)
	}
}

func TestOpen_plainHTTP_rejectsNonLocalhost(t *testing.T) {
	t.Parallel()

	opts := analytics.Options{APIKey: "test-key", Endpoint: "http://example.com"}
	_, err := New(opts)

	var invalid *analytics.InvalidOptionsError
	if !errors.As(err, &invalid) {
		t.Fatalf("want *InvalidOptionsError, got %v", err)
	}
}

func TestOpen_httpsEndpoint_acceptsSecureURL(t *testing.T) {
	t.Parallel()

	a, err := New(analytics.Options{APIKey: "test-key", Endpoint: "https://example.com"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestOpen_defaults_appliesAnonymousAndGroup(t *testing.T) {
	t.Parallel()

	a, err := New(analytics.Options{APIKey: "test-key"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	ad, ok := a.(*adapter)
	if !ok {
		t.Fatalf("want *adapter, got %T", a)
	}

	if ad.anonymousID != analytics.DefaultAnonymousID {
		t.Fatalf("anonymousID: got %q, want %q", ad.anonymousID, analytics.DefaultAnonymousID)
	}

	if ad.groupType != analytics.DefaultGroupType {
		t.Fatalf("groupType: got %q, want %q", ad.groupType, analytics.DefaultGroupType)
	}
}

func TestOpen_options_appliesCustomAnonymousAndGroup(t *testing.T) {
	t.Parallel()

	opts := analytics.Options{APIKey: "test-key", AnonymousID: "guest", GroupType: "workspace"}

	a, err := New(opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Fatalf("close: %v", err)
		}
	})

	ad, ok := a.(*adapter)
	if !ok {
		t.Fatalf("want *adapter, got %T", a)
	}

	if ad.anonymousID != "guest" {
		t.Fatalf("anonymousID: got %q, want guest", ad.anonymousID)
	}

	if ad.groupType != "workspace" {
		t.Fatalf("groupType: got %q, want workspace", ad.groupType)
	}
}

func TestClose_success_returnsNil(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if f.closes != 1 {
		t.Fatalf("closes: got %d, want 1", f.closes)
	}
}

func TestClose_error_wrapsCause(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	fakeErr := errors.New("boom")
	f.closeErr = fakeErr

	if err := a.Close(); !errors.Is(err, fakeErr) {
		t.Fatalf("want wrapped boom, got %v", err)
	}
}

func TestOperations_canceledContext_returnsCanceled(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := a.Track(ctx, "event", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("track: want context.Canceled, got %v", err)
	}

	if err := a.Identify(ctx, "u1", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("identify: want context.Canceled, got %v", err)
	}

	if err := a.Group(ctx, "u1", "org-1", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("group: want context.Canceled, got %v", err)
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestTrack_tooManyProperties_returnsCountLimit(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.maxProperties = 2

	props := map[string]any{"a": "1", "b": "2", "c": "3"}
	if err := a.Track(t.Context(), "event", props); err == nil {
		t.Fatal("want error for too many properties, got nil")
	} else {
		var countErr *analytics.CountLimitError
		if !errors.As(err, &countErr) {
			t.Fatalf("want *CountLimitError, got %v", err)
		}
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestTrack_oversizedProperties_returnsSizeLimit(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	props := map[string]any{"big": strings.Repeat("x", 128*1024)}
	if err := a.Track(t.Context(), "event", props); err == nil {
		t.Fatal("want error for oversized properties, got nil")
	} else {
		var sizeErr *analytics.SizeLimitError
		if !errors.As(err, &sizeErr) {
			t.Fatalf("want *SizeLimitError, got %v", err)
		}
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestIdentify_oversizedTraits_returnsSizeLimit(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	traits := map[string]any{"big": strings.Repeat("x", 128*1024)}
	if err := a.Identify(t.Context(), "u1", traits); err == nil {
		t.Fatal("want error for oversized traits, got nil")
	} else {
		var sizeErr *analytics.SizeLimitError
		if !errors.As(err, &sizeErr) {
			t.Fatalf("want *SizeLimitError, got %v", err)
		}
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestGroup_oversizedTraits_returnsSizeLimit(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	traits := map[string]any{"big": strings.Repeat("x", 128*1024)}
	if err := a.Group(t.Context(), "u1", "g1", traits); err == nil {
		t.Fatal("want error for oversized traits, got nil")
	} else {
		var sizeErr *analytics.SizeLimitError
		if !errors.As(err, &sizeErr) {
			t.Fatalf("want *SizeLimitError, got %v", err)
		}
	}

	if got := f.count(); got != 0 {
		t.Fatalf("want zero enqueues, got %d", got)
	}
}

func TestTrack_concurrent_enqueuesAllMessages(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)

	var wg sync.WaitGroup

	for i := range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			ctx := analytics.WithUserID(t.Context(), "u1")
			if err := a.Track(ctx, "event", map[string]any{"i": i}); err != nil {
				t.Error(err)
			}
		}()
	}

	wg.Wait()

	if got := f.count(); got != 20 {
		t.Fatalf("want 20 enqueues, got %d", got)
	}
}

func TestOpen_clientError_wrapsCause(t *testing.T) {
	// Sequential on purpose: it mutates the newClient seam used by Open.
	old := newClient
	newClient = func(string, posthog.Config) (posthog.Client, error) {
		return nil, errFakeClient
	}
	t.Cleanup(func() { newClient = old })

	if _, err := New(analytics.Options{APIKey: "test-key"}); !errors.Is(err, errFakeClient) {
		t.Fatalf("want wrapped client error, got %v", err)
	}
}

func TestCheckEndpoint_malformedURL_rejectsOptions(t *testing.T) {
	t.Parallel()

	if err := checkEndpoint("http://[::1"); err == nil {
		t.Fatal("want error for malformed endpoint, got nil")
	} else {
		var invalid *analytics.InvalidOptionsError
		if !errors.As(err, &invalid) {
			t.Fatalf("want *InvalidOptionsError, got %v", err)
		}
	}
}

func TestOpen_localhostHTTP_acceptsLocalURL(t *testing.T) {
	t.Parallel()

	a, err := New(analytics.Options{APIKey: "test-key", Endpoint: "http://localhost:9"})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestGroup_emptyType_fallsBackToDefault(t *testing.T) {
	t.Parallel()

	a, f := stubAdapter(t)
	a.groupType = ""

	if err := a.Group(t.Context(), "u1", "org-1", nil); err != nil {
		t.Fatal(err)
	}

	msg, ok := f.first().(posthog.GroupIdentify)
	if !ok {
		t.Fatalf("want GroupIdentify, got %T", f.first())
	}

	if msg.Type != analytics.DefaultGroupType {
		t.Fatalf("Type: got %q, want %q", msg.Type, analytics.DefaultGroupType)
	}
}

func TestOpen_endpoint_wiresLocalServer(t *testing.T) {
	t.Parallel()

	var hits atomic.Int64

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/batch") {
			hits.Add(1)
		}

		_, _ = io.Copy(io.Discard, r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a, err := New(analytics.Options{APIKey: "test-key", Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := a.Track(t.Context(), "signed_up", map[string]any{"plan": "free"}); err != nil {
		t.Fatalf("track: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if got := hits.Load(); got == 0 {
		t.Fatal("want at least one /batch hit, got zero")
	}
}

func TestOpen_endpoint_deliversTrackPayload(t *testing.T) {
	t.Parallel()

	var (
		mu        sync.Mutex
		rawBodies []json.RawMessage
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		var batch struct {
			Batch []json.RawMessage `json:"batch"`
		}

		_ = json.Unmarshal(body, &batch)

		mu.Lock()
		rawBodies = append(rawBodies, batch.Batch...)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	a, err := New(analytics.Options{APIKey: "test-key", Endpoint: srv.URL})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := a.Track(t.Context(), "signed_up", map[string]any{"plan": "free"}); err != nil {
		t.Fatalf("track: %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()

	if len(rawBodies) == 0 {
		t.Fatal("want at least one event, got zero")
	}

	var evt map[string]any
	if err := json.Unmarshal(rawBodies[0], &evt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if evt["event"] != "signed_up" {
		t.Fatalf("event: got %v, want signed_up", evt["event"])
	}
}
