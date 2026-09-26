package log

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/zenta-dev/zever/analytics"
)

// lockedWriter serializes writes into buf.
type lockedWriter struct {
	mu  sync.Mutex
	buf *bytes.Buffer
}

// Write appends p to the buffer under lock.
func (w *lockedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.buf.Write(p)
}

// newTestAdapter returns an adapter logging JSON lines into buf.
func newTestAdapter(buf *bytes.Buffer) *adapter {
	return &adapter{
		logger:             slog.New(slog.NewJSONHandler(buf, nil)),
		anonymousID:        analytics.DefaultAnonymousID,
		maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
	}
}

// flakyValue marshals fine once, then fails, so checkBounds passes but the
// single-pass MarshalValidated remarshal fails.
type flakyValue struct {
	calls *int
}

// MarshalJSON succeeds on the first call and fails after.
func (f flakyValue) MarshalJSON() ([]byte, error) {
	*f.calls++

	if *f.calls > 1 {
		return nil, errors.New("boom")
	}

	return []byte(`"ok"`), nil
}

// decodeEntry unmarshals a single JSON log line.
func decodeEntry(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal log entry: %v", err)
	}

	return entry
}

func TestOpen_zeroOptions_succeeds(t *testing.T) {
	t.Parallel()

	a, err := New(analytics.Options{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if a == nil {
		t.Fatal("expected non-nil adapter")
	}
}

func TestOpen_invalidOptions_returnsInvalidOptions(t *testing.T) {
	t.Parallel()

	cases := map[string]analytics.Options{
		"negative bytes": {MaxPropertiesBytes: -1},
		"negative count": {MaxProperties: -1},
	}

	for name, opts := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := New(opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}

			if !errors.Is(err, analytics.ErrInvalidOptions) {
				t.Errorf("expected ErrInvalidOptions, got %v", err)
			}
		})
	}
}

func TestTrack_roundtrip_writesJSONFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a := newTestAdapter(&buf)

	if err := a.Track(t.Context(), "page_view", map[string]any{"page": "/home"}); err != nil {
		t.Fatalf("track: %v", err)
	}

	entry := decodeEntry(t, &buf)
	if entry["event"] != "page_view" {
		t.Errorf("want event=page_view, got %v", entry["event"])
	}

	if entry["user_id"] != analytics.DefaultAnonymousID {
		t.Errorf("want user_id=%q, got %v", analytics.DefaultAnonymousID, entry["user_id"])
	}
}

func TestTrack_ctxUser_fallsBackToContextID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a := newTestAdapter(&buf)
	ctx := analytics.WithUserID(t.Context(), "u1")

	if err := a.Track(ctx, "page_view", nil); err != nil {
		t.Fatalf("track: %v", err)
	}

	if entry := decodeEntry(t, &buf); entry["user_id"] != "u1" {
		t.Errorf("want user_id=u1, got %v", entry["user_id"])
	}
}

func TestIdentity_missingIdentity_returnsSentinel(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*adapter) error{
		"track": func(a *adapter) error { return a.Track(t.Context(), "page_view", nil) },
		"identify": func(a *adapter) error {
			return a.Identify(t.Context(), "", map[string]any{"email": "a@b.com"})
		},
		"group": func(a *adapter) error { return a.Group(t.Context(), "", "org-1", nil) },
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := &adapter{logger: slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))}
			if err := call(a); !errors.Is(err, analytics.ErrMissingIdentity) {
				t.Errorf("expected ErrMissingIdentity, got %v", err)
			}
		})
	}
}

func TestGroup_emptyGroupID_returnsSentinel(t *testing.T) {
	t.Parallel()

	a := &adapter{
		logger:      slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		anonymousID: analytics.DefaultAnonymousID,
	}

	if err := a.Group(t.Context(), "u1", "", nil); !errors.Is(err, analytics.ErrMissingGroupID) {
		t.Errorf("expected ErrMissingGroupID, got %v", err)
	}
}

func TestIdentify_roundtrip_writesUserID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a := newTestAdapter(&buf)

	if err := a.Identify(t.Context(), "user-1", map[string]any{"email": "a@b.com"}); err != nil {
		t.Fatalf("identify: %v", err)
	}

	if entry := decodeEntry(t, &buf); entry["user_id"] != "user-1" {
		t.Errorf("want user_id=user-1, got %v", entry["user_id"])
	}
}

func TestIdentify_emptyArg_fallsBackToExpectedID(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		ctxUser string
		want    string
	}{
		"ctx user":  {ctxUser: "u1", want: "u1"},
		"anonymous": {want: analytics.DefaultAnonymousID},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			a := newTestAdapter(&buf)
			ctx := t.Context()
			if tc.ctxUser != "" {
				ctx = analytics.WithUserID(ctx, tc.ctxUser)
			}

			if err := a.Identify(ctx, "", map[string]any{"email": "a@b.com"}); err != nil {
				t.Fatalf("identify: %v", err)
			}

			if entry := decodeEntry(t, &buf); entry["user_id"] != tc.want {
				t.Errorf("want user_id=%s, got %v", tc.want, entry["user_id"])
			}
		})
	}
}

func TestGroup_roundtrip_writesGroupID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	a := newTestAdapter(&buf)

	if err := a.Group(t.Context(), "user-1", "org-42", map[string]any{"plan": "pro"}); err != nil {
		t.Fatalf("group: %v", err)
	}

	entry := decodeEntry(t, &buf)
	if entry["group_id"] != "org-42" {
		t.Errorf("want group_id=org-42, got %v", entry["group_id"])
	}

	if entry["user_id"] != "user-1" {
		t.Errorf("want user_id=user-1, got %v", entry["user_id"])
	}
}

func TestGroup_emptyArg_fallsBackToExpectedID(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		ctxUser string
		want    string
	}{
		"ctx user":  {ctxUser: "u1", want: "u1"},
		"anonymous": {want: analytics.DefaultAnonymousID},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			a := newTestAdapter(&buf)
			ctx := t.Context()
			if tc.ctxUser != "" {
				ctx = analytics.WithUserID(ctx, tc.ctxUser)
			}

			if err := a.Group(ctx, "", "org-1", nil); err != nil {
				t.Fatalf("group: %v", err)
			}

			if entry := decodeEntry(t, &buf); entry["user_id"] != tc.want {
				t.Errorf("want user_id=%s, got %v", tc.want, entry["user_id"])
			}
		})
	}
}

func TestCalls_cyclicMap_returnsMarshalError(t *testing.T) {
	t.Parallel()

	cyclic := func() map[string]any {
		m := map[string]any{}
		m["self"] = m

		return m
	}

	cases := map[string]func(*adapter) error{
		"track":    func(a *adapter) error { return a.Track(t.Context(), "page_view", cyclic()) },
		"identify": func(a *adapter) error { return a.Identify(t.Context(), "user-1", cyclic()) },
		"group":    func(a *adapter) error { return a.Group(t.Context(), "user-1", "org-42", cyclic()) },
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := &adapter{
				logger:             slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
				anonymousID:        analytics.DefaultAnonymousID,
				maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
			}
			if err := call(a); err == nil {
				t.Fatal("expected marshal error, got nil")
			}
		})
	}
}

func TestCalls_remarshalFailure_returnsError(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*adapter, map[string]any) error{
		"track": func(a *adapter, m map[string]any) error {
			return a.Track(t.Context(), "event", m)
		},
		"identify": func(a *adapter, m map[string]any) error {
			return a.Identify(t.Context(), "user-1", m)
		},
		"group": func(a *adapter, m map[string]any) error {
			return a.Group(t.Context(), "user-1", "org-42", m)
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			a := &adapter{
				logger:             slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
				anonymousID:        analytics.DefaultAnonymousID,
				maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
			}
			calls := 0
			props := map[string]any{"f": flakyValue{calls: &calls}}

			if err := call(a, props); err == nil {
				t.Fatal("expected remarshal error, got nil")
			}
		})
	}
}

func TestTrack_oversizedProperties_returnsSizeLimitError(t *testing.T) {
	t.Parallel()

	a := &adapter{
		logger:             slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		anonymousID:        analytics.DefaultAnonymousID,
		maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
	}
	props := map[string]any{"big": strings.Repeat("x", 128*1024)}

	err := a.Track(t.Context(), "event", props)
	if err == nil {
		t.Fatal("expected error for oversized properties, got nil")
	}

	var sizeErr *analytics.SizeLimitError
	if !errors.As(err, &sizeErr) {
		t.Errorf("expected *SizeLimitError, got %T: %v", err, err)
	}
}

func TestTrack_tooManyProperties_returnsCountLimitError(t *testing.T) {
	t.Parallel()

	a := &adapter{
		logger:             slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)),
		anonymousID:        analytics.DefaultAnonymousID,
		maxProperties:      2,
		maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
	}
	props := map[string]any{"a": "1", "b": "2", "c": "3"}

	err := a.Track(t.Context(), "event", props)
	if err == nil {
		t.Fatal("expected error for too many properties, got nil")
	}

	var countErr *analytics.CountLimitError
	if !errors.As(err, &countErr) {
		t.Errorf("expected *CountLimitError, got %T: %v", err, err)
	}
}

func TestCalls_cancelledContext_returnsCtxError(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*adapter, context.Context) error{
		"track":    func(a *adapter, ctx context.Context) error { return a.Track(ctx, "e", nil) },
		"identify": func(a *adapter, ctx context.Context) error { return a.Identify(ctx, "u1", nil) },
		"group": func(a *adapter, ctx context.Context) error {
			return a.Group(ctx, "u1", "org-1", nil)
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer

			a := newTestAdapter(&buf)
			ctx, cancel := context.WithCancel(analytics.WithUserID(t.Context(), "u1"))
			cancel()

			if err := call(a, ctx); !errors.Is(err, context.Canceled) {
				t.Errorf("expected context.Canceled, got %v", err)
			}
		})
	}
}

func TestTrack_concurrent_writesWithoutRace(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer

	w := &lockedWriter{buf: &buf}
	a := &adapter{
		logger:             slog.New(slog.NewJSONHandler(w, nil)),
		anonymousID:        analytics.DefaultAnonymousID,
		maxPropertiesBytes: analytics.DefaultMaxPropertiesBytes,
	}

	var wg sync.WaitGroup

	for range 20 {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if err := a.Track(t.Context(), "event", map[string]any{"k": "v"}); err != nil {
				t.Errorf("track: %v", err)
			}
		}()
	}

	wg.Wait()

	if got := bytes.Count(buf.Bytes(), []byte("\n")); got != 20 {
		t.Errorf("want 20 log lines, got %d", got)
	}
}

func TestClose_noop_returnsNil(t *testing.T) {
	t.Parallel()

	a := newTestAdapter(&bytes.Buffer{})
	if err := a.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
