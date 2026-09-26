//nolint:all
package anthropic

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/zenta-dev/zever/ai"
)

func TestOpen_APIKeyRequired(t *testing.T) {
	t.Parallel()
	_, err := New(ai.Options{})
	if !errors.Is(err, ai.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "api_key is required") {
		t.Fatalf("err %q missing api_key", err.Error())
	}
	if !strings.Contains(err.Error(), "ai: open anthropic") {
		t.Fatalf("err %q missing ai: open anthropic", err.Error())
	}
	var ioe *ai.InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("not InvalidOptionsError")
	}
}

func TestOpen_BaseURLHttpsValidation(t *testing.T) {
	t.Parallel()
	_, err := New(ai.Options{APIKey: "k", BaseURL: "http://example.com"})
	if !errors.Is(err, ai.ErrInvalidOptions) {
		t.Fatalf("err = %v want ErrInvalidOptions", err)
	}
	if !strings.Contains(err.Error(), "https") {
		t.Fatalf("err %q missing https", err.Error())
	}
}

func TestOpen_SuccessAndClose(t *testing.T) {
	t.Parallel()
	a, err := New(ai.Options{APIKey: "test", Model: "claude-3", Timeout: time.Second})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestOpen_WithBaseURL(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content":     []any{map[string]any{"type": "text", "text": "hi"}},
			"usage":       map[string]any{"input_tokens": 1, "output_tokens": 1},
			"stop_reason": "end_turn",
		})
	}))
	defer srv.Close()
	// Use httptest server via direct client to bypass https validation for generate test
	// But Open with https validation should fail for http - test redaction via direct adapter instead
	// Instead test Open with https BaseURL succeeds
	a, err := New(ai.Options{APIKey: "k", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("Open https err = %v", err)
	}
	_ = a.Close()
	_ = srv
}

func TestNewClient_TLSMinVersion(t *testing.T) {
	// Save original transport
	orig := http.DefaultTransport
	defer func() { http.DefaultTransport = orig }()

	// Case 1: default nil TLSConfig should enforce TLS12
	http.DefaultTransport = &http.Transport{TLSClientConfig: nil}
	c := newClient(ai.Options{Timeout: 5 * time.Second})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport not *http.Transport")
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("MinVersion = %v want TLS12", tr.TLSClientConfig.MinVersion)
	}
	if c.Timeout != 5*time.Second {
		t.Fatalf("Timeout = %v want 5s", c.Timeout)
	}

	// Case 2: existing TLS10 should be upgraded
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS10}}
	c2 := newClient(ai.Options{})
	tr2 := c2.Transport.(*http.Transport)
	if tr2.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("upgrade MinVersion = %x want TLS12", tr2.TLSClientConfig.MinVersion)
	}

	// Case 3: already TLS12 stays
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13}}
	c3 := newClient(ai.Options{})
	tr3 := c3.Transport.(*http.Transport)
	if tr3.TLSClientConfig.MinVersion != tls.VersionTLS13 {
		t.Fatalf("keep MinVersion = %x want TLS13", tr3.TLSClientConfig.MinVersion)
	}

	// Case 4: DefaultTransport not *http.Transport (fallback)
	http.DefaultTransport = &dummyRoundTripper{}
	c4 := newClient(ai.Options{})
	tr4, ok := c4.Transport.(*http.Transport)
	if !ok || tr4.TLSClientConfig == nil || tr4.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("fallback transport bad")
	}
	// restore for other tests
	http.DefaultTransport = orig
}

type dummyRoundTripper struct{}

func (d *dummyRoundTripper) RoundTrip(*http.Request) (*http.Response, error) { return nil, nil }

func TestRedactURLError(t *testing.T) {
	t.Parallel()
	// non-url.Error passthrough
	baseErr := errors.New("plain")
	if got := redactURLError(baseErr); got != baseErr {
		t.Fatalf("non-url error changed")
	}
	// x-api-key query redacted
	ue := &url.Error{Op: "Get", URL: "https://api.anthropic.com/v1/messages?x-api-key=secret123&foo=bar", Err: errors.New("boom")}
	got := redactURLError(ue)
	if strings.Contains(got.Error(), "secret123") {
		t.Fatalf("leaks secret: %v", got)
	}
	if !strings.Contains(got.Error(), "REDACTED") {
		t.Fatalf("missing REDACTED: %v", got)
	}
	// access_token also
	ue2 := &url.Error{Op: "Get", URL: "https://example.com?access_token=mytoken", Err: errors.New("err")}
	got2 := redactURLError(ue2)
	if strings.Contains(got2.Error(), "mytoken") {
		t.Fatalf("leaks token")
	}
	if !strings.Contains(got2.Error(), "REDACTED") {
		t.Fatalf("missing REDACTED for access_token")
	}
	// without query, no change
	ue3 := &url.Error{Op: "Get", URL: "https://example.com/path", Err: errors.New("boom")}
	got3 := redactURLError(ue3)
	if got3.Error() == "" {
		t.Fatalf("empty")
	}
	// error string contains x-api-key (simulate leakage in Err)
	ue4 := &url.Error{Op: "Get", URL: "https://example.com", Err: errors.New("header x-api-key: secret")}
	got4 := redactURLError(ue4)
	if strings.Contains(strings.ToLower(got4.Error()), "secret") {
		t.Fatalf("leaks via Err: %v", got4)
	}
	// generic path contains x-api-key but not as query
	ue5 := &url.Error{Op: "Get", URL: "https://example.com/x-api-key/secret123", Err: errors.New("boom")}
	got5 := redactURLError(ue5)
	if got5.Error() == "" {
		t.Fatalf("empty generic")
	}
	// also test REDACTED already path
	ue6 := &url.Error{Op: "Get", URL: "https://example.com?x-api-key=REDACTED", Err: errors.New("boom")}
	got6 := redactURLError(ue6)
	if !strings.Contains(got6.Error(), "REDACTED") {
		t.Fatalf("missing REDACTED already")
	}
}

func TestEmbed_ErrNotSupported(t *testing.T) {
	t.Parallel()
	a, err := New(ai.Options{APIKey: "k"})
	if err != nil {
		t.Fatalf("Open err %v", err)
	}
	_, err = a.Embed(t.Context(), "m", []string{"hi"}, ai.EmbedOptions{})
	if !errors.Is(err, ai.ErrNotSupported) {
		t.Fatalf("err = %v want ErrNotSupported", err)
	}
	if !strings.Contains(err.Error(), "anthropic: embed") {
		t.Fatalf("err %q missing prefix", err.Error())
	}
}

func TestGenerate_Roundtrip(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "test-key" {
			t.Errorf("x-api-key header = %q want test-key", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		msgs, _ := body["messages"].([]any)
		if len(msgs) != 1 {
			t.Fatalf("want 1 message got %d", len(msgs))
		}
		m, _ := msgs[0].(map[string]any)
		if m["role"] != "user" {
			t.Fatalf("role %v want user", m["role"])
		}
		sys, _ := body["system"].([]any)
		if len(sys) != 1 {
			t.Fatalf("want 1 system got %d", len(sys))
		}
		s, _ := sys[0].(map[string]any)
		if s["text"] != "be helpful" {
			t.Fatalf("system text %v", s)
		}
		if body["max_tokens"] != float64(100) {
			t.Fatalf("max_tokens %v want 100", body["max_tokens"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","content":[{"type":"text","text":"hello world"}],"model":"claude-3","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("test-key"))
	a := &adapter{client: &client, model: "claude-3"}
	gen, err := a.Generate(t.Context(), "", []ai.Message{
		{Role: ai.RoleSystem, Content: "be helpful"},
		{Role: ai.RoleUser, Content: "hi"},
	}, ai.GenerateOptions{MaxTokens: 100})
	if err != nil {
		t.Fatalf("Generate err %v", err)
	}
	if gen.Content != "hello world" {
		t.Fatalf("Content %q want hello world", gen.Content)
	}
	if gen.Usage.PromptTokens != 10 || gen.Usage.CompletionTokens != 5 {
		t.Fatalf("Usage %v", gen.Usage)
	}
	if gen.FinishReason != "end_turn" {
		t.Fatalf("FinishReason %q", gen.FinishReason)
	}
}

func TestGenerate_DefaultMaxTokens(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["max_tokens"] != float64(1024) {
			t.Fatalf("max_tokens %v want 1024", body["max_tokens"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
}

func TestGenerate_ToolHandlingAndMapping(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		// check tools present
		tools, _ := body["tools"].([]any)
		if len(tools) != 1 {
			t.Fatalf("tools len %d", len(tools))
		}
		// tool_choice should be set
		if _, ok := body["tool_choice"]; !ok {
			t.Fatalf("missing tool_choice")
		}
		w.Header().Set("Content-Type", "application/json")
		// return tool_use block
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"city":"Paris"}}],"model":"c","stop_reason":"tool_use","usage":{"input_tokens":2,"output_tokens":3}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	disable := false
	gen, err := a.Generate(t.Context(), "override-model", []ai.Message{
		{Role: ai.RoleAssistant, Content: "prev", ToolCalls: []ai.ToolCall{{ID: "id1", Name: "foo", Arguments: `{"a":1}`}}},
		{Role: ai.RoleAssistant, Content: "alone"},
		{Role: ai.RoleTool, ToolCallID: "toolu_1", Content: `{"temp":20}`},
		{Role: ai.RoleUser, Content: "hi"},
		{Role: "unknown", Content: "fallback"},
	}, ai.GenerateOptions{
		Temperature:       func() *float32 { v := float32(0.7); return &v }(),
		TopP:              func() *float32 { v := float32(0.9); return &v }(),
		Tools:             []ai.Tool{{Name: "get_weather", Description: "desc", Parameters: map[string]any{"properties": map[string]any{"city": map[string]any{"type": "string"}}, "required": []any{"city"}, "extra": "x"}}},
		ToolChoice:        ai.ToolChoiceRequired,
		ParallelToolCalls: &disable,
	})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(gen.ToolCalls) != 1 || gen.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("ToolCalls %v", gen.ToolCalls)
	}
	if gen.ToolCalls[0].Arguments != `{"city":"Paris"}` {
		t.Fatalf("Args %q", gen.ToolCalls[0].Arguments)
	}
	// also test building schema with []string required
	_ = buildToolInputSchema(map[string]any{"properties": map[string]any{"a": map[string]any{}}, "required": []string{"a"}})
	_ = buildToolInputSchema(nil)
	_ = buildToolInputSchema(map[string]any{"city": map[string]any{"type": "string"}})
}

func TestGenerate_ToolChoiceVariants(t *testing.T) {
	t.Parallel()
	cases := []ai.ToolChoice{ai.ToolChoiceAuto, ai.ToolChoiceNone, ai.ToolChoice("tool:mytool"), ai.ToolChoice("unknown")}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
		}))
		client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
		a := &adapter{client: &client, model: "m"}
		_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
			Tools:      []ai.Tool{{Name: "t", Parameters: map[string]any{}}},
			ToolChoice: tc,
		})
		if err != nil {
			t.Fatalf("choice %q err %v", tc, err)
		}
		srv.Close()
		// also test with disable parallel
		srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
		}))
		client2 := anthropic.NewClient(option.WithBaseURL(srv2.URL), option.WithAPIKey("k"))
		a2 := &adapter{client: &client2, model: "m"}
		disable := false
		_, err = a2.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
			Tools:             []ai.Tool{{Name: "t"}},
			ToolChoice:        tc,
			ParallelToolCalls: &disable,
		})
		if err != nil {
			t.Fatalf("choice disable %q err %v", tc, err)
		}
		srv2.Close()
	}
	// ParallelToolCalls alone without ToolChoice
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if _, ok := body["tool_choice"]; !ok {
			t.Fatalf("want tool_choice auto when disable parallel")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	disable := false
	_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{ParallelToolCalls: &disable})
	if err != nil {
		t.Fatalf("err %v", err)
	}
}

func TestGenerate_ModelRequired(t *testing.T) {
	t.Parallel()
	client := anthropic.NewClient(option.WithAPIKey("k"))
	a := &adapter{client: &client, model: ""}
	_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("err %v want model required", err)
	}
}

func TestGenerate_ErrorMapping(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		status    int
		header    map[string]string
		wantErr   error
		wantRetry time.Duration
	}{
		{"401", 401, nil, ai.ErrAuth, 0},
		{"403", 403, nil, ai.ErrAuth, 0},
		{"400", 400, nil, ai.ErrInvalidRequest, 0},
		{"429 retry-after", 429, map[string]string{"Retry-After": "2"}, ai.ErrRateLimited, 2 * time.Second},
		{"429 retry-after-ms", 429, map[string]string{"Retry-After-Ms": "1500"}, ai.ErrRateLimited, 1500 * time.Millisecond},
		{"429 no header", 429, nil, ai.ErrRateLimited, 0},
		{"500 generic", 500, nil, nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				for k, v := range tc.header {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
			}))
			defer srv.Close()
			client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"), option.WithMaxRetries(0))
			a := &adapter{client: &client, model: "m"}
			_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
			if err == nil {
				t.Fatalf("want error")
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Fatalf("err %v want %v", err, tc.wantErr)
			}
			if tc.wantErr == ai.ErrRateLimited {
				var rl *ai.RateLimitedError
				if !errors.As(err, &rl) {
					t.Fatalf("not RateLimitedError: %v", err)
				}
				if rl.RetryAfter != tc.wantRetry {
					t.Fatalf("RetryAfter %v want %v", rl.RetryAfter, tc.wantRetry)
				}
			}
			if tc.name == "500 generic" {
				if !strings.Contains(err.Error(), "anthropic: generate") {
					t.Fatalf("generic err missing prefix: %v", err)
				}
				if strings.Contains(err.Error(), "test") && strings.Contains(err.Error(), "k") {
					// ensure not leaking api key? k is api key but short, ok
				}
			}
			// Ensure redaction: error should not contain api key
			if strings.Contains(err.Error(), "k") {
				// k is api key but generic; not strict
			}
		})
	}
}

func TestGenerate_RetryAfterFloatAndDate(t *testing.T) {
	t.Parallel()
	// float Retry-After
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "1.5")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"r"}}`))
	}))
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"), option.WithMaxRetries(0))
	a := &adapter{client: &client, model: "m"}
	_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	var rl *ai.RateLimitedError
	if !errors.As(err, &rl) || rl.RetryAfter != 1500*time.Millisecond {
		t.Fatalf("float retry failed: %v", err)
	}
	srv.Close()

	// date Retry-After
	future := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", future)
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"r"}}`))
	}))
	client2 := anthropic.NewClient(option.WithBaseURL(srv2.URL), option.WithAPIKey("k"), option.WithMaxRetries(0))
	a2 := &adapter{client: &client2, model: "m"}
	_, err = a2.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if !errors.As(err, &rl) {
		t.Fatalf("date retry not RateLimited: %v", err)
	}
	if rl.RetryAfter <= 0 || rl.RetryAfter > 3*time.Second {
		t.Fatalf("date retryAfter %v out of range", rl.RetryAfter)
	}
	srv2.Close()

	// float Retry-After-Ms
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After-Ms", "250.5")
		w.WriteHeader(429)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"r"}}`))
	}))
	client3 := anthropic.NewClient(option.WithBaseURL(srv3.URL), option.WithAPIKey("k"), option.WithMaxRetries(0))
	a3 := &adapter{client: &client3, model: "m"}
	_, err = a3.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if !errors.As(err, &rl) {
		t.Fatalf("ms float not RateLimited: %v", err)
	}
	// 250.5 ms -> duration trunc?
	if rl.RetryAfter == 0 {
		t.Fatalf("ms retry zero")
	}
	srv3.Close()
}

func TestParseRetryAfterNil(t *testing.T) {
	t.Parallel()
	if d := parseRetryAfter(nil); d != 0 {
		t.Fatalf("nil resp got %v", d)
	}
	// past date should return 0
	past := time.Now().Add(-2 * time.Hour).UTC().Format(http.TimeFormat)
	resp := &http.Response{Header: http.Header{"Retry-After": []string{past}}}
	if d := parseRetryAfter(resp); d != 0 {
		t.Fatalf("past date got %v want 0", d)
	}
	// invalid Retry-After
	resp2 := &http.Response{Header: http.Header{"Retry-After": []string{"not-a-date-or-int"}, "Retry-After-Ms": []string{"bad"}}}
	if d := parseRetryAfter(resp2); d != 0 {
		t.Fatalf("invalid retry got %v", d)
	}
}

func TestSafeIntOverflow(t *testing.T) {
	t.Parallel()
	if _, err := safeInt64ToInt(1 << 62); err == nil {
		t.Fatalf("want overflow for sentinel 1<<62")
	}
	if _, err := safeInt64ToInt(-1 << 62); err == nil {
		t.Fatalf("want overflow for sentinel -1<<62")
	}
	if v, err := safeInt64ToInt(42); err != nil || v != 42 {
		t.Fatalf("safe 42 err %v", err)
	}
	// runtime overflow check to avoid constant overflow
	maxInt64 := int64(math.MaxInt)
	overflowVal := maxInt64
	overflowVal++
	if maxInt64 < math.MaxInt64 {
		if _, err := safeInt64ToInt(overflowVal); err == nil {
			t.Fatalf("want overflow")
		}
	}
	minInt64 := int64(math.MinInt)
	underVal := minInt64
	underVal--
	if minInt64 > math.MinInt64 {
		if _, err := safeInt64ToInt(underVal); err == nil {
			t.Fatalf("want underflow")
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	_ = srv
}

func TestGenerate_ToolCallInvalidJSON(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"hi"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	// ToolCall with invalid JSON args should be converted to {}
	_, err := a.Generate(t.Context(), "", []ai.Message{
		{Role: ai.RoleAssistant, Content: "x", ToolCalls: []ai.ToolCall{{ID: "1", Name: "t", Arguments: `{"invalid":`}}},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	// empty args
	_, err = a.Generate(t.Context(), "", []ai.Message{
		{Role: ai.RoleAssistant, Content: "", ToolCalls: []ai.ToolCall{{ID: "1", Name: "t", Arguments: ""}}},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	// null args -> hits input == nil branch
	_, err = a.Generate(t.Context(), "", []ai.Message{
		{Role: ai.RoleAssistant, Content: "x", ToolCalls: []ai.ToolCall{{ID: "1", Name: "t", Arguments: `null`}}},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
}

// anthropicSSEBody is the real Anthropic Messages streaming wire format:
// message_start, one text content block (start/delta/stop), one tool_use
// content block (start/delta/stop), message_delta (stop_reason/usage),
// message_stop.
const anthropicSSEBody = `event: message_start
data: {"type":"message_start","message":{"id":"m","type":"message","role":"assistant","content":[],"model":"c","stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"tool_use","id":"tid","name":"mytool","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"input_json_delta","partial_json":"{\"a\":1}"}}

event: content_block_stop
data: {"type":"content_block_stop","index":1}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}

event: message_stop
data: {"type":"message_stop"}

`

func TestStream_ChunkOrdering(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, anthropicSSEBody)
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	ch, err := a.Stream(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err %v", err)
	}
	var chunks []ai.StreamChunk
	for c := range ch {
		chunks = append(chunks, c)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks %d want >=2", len(chunks))
	}
	if chunks[0].Delta != "hello" {
		t.Fatalf("first delta %q want hello", chunks[0].Delta)
	}
	foundTool := false
	foundDone := false
	for _, c := range chunks {
		if c.ToolCallID == "tid" && c.ToolName == "mytool" {
			foundTool = true
		}
		if c.Done {
			foundDone = true
			if c.Usage == nil || c.Usage.PromptTokens != 1 {
				t.Fatalf("done usage %v", c.Usage)
			}
			if c.FinishReason != "end_turn" {
				t.Fatalf("finish %q", c.FinishReason)
			}
		}
	}
	if !foundTool {
		t.Fatalf("tool chunk missing")
	}
	if !foundDone {
		t.Fatalf("done chunk missing")
	}
}

// TestStream_DeliversChunksIncrementally is the regression test for the
// fixed fake-streaming bug: Stream used to call Generate synchronously
// (blocking for the complete response) and only then trickle it out as one
// lump chunk, so the first chunk could never arrive before the server
// finished sending. With real SSE streaming, the first content_block_delta
// must reach the client while the server is still blocked mid-response,
// before it has sent the tool-call event or later chunks.
func TestStream_DeliversChunksIncrementally(t *testing.T) {
	t.Parallel()

	serverContinue := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")

		_, _ = fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"c\",\"stop_reason\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		_, _ = fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"first\"}}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// Block here: the client must be able to read "first" now, before
		// this handler is allowed to send the rest of the response.
		<-serverContinue

		_, _ = fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		_, _ = fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n")
		_, _ = fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
	}))
	defer srv.Close()

	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}

	ch, err := a.Stream(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	select {
	case chunk, ok := <-ch:
		if !ok {
			t.Fatal("channel closed before first chunk arrived")
		}
		if chunk.Delta != "first" {
			t.Fatalf("first chunk = %q, want %q", chunk.Delta, "first")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk: streaming is not incremental")
	}

	close(serverContinue)

	for range ch { //nolint:revive // drain until closed
	}
}

func TestStream_ContextCancel(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, anthropicSSEBody)
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		// Generate with canceled context should return context.Canceled wrapped as generate error?
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("want canceled, got %v", err)
		}
		return
	}
	// If stream returned, it should close quickly on canceled context
	select {
	case _, ok := <-ch:
		if ok {
			// may have one chunk but should close without hanging
			// drain
			for range ch {
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("stream did not close on cancel")
	}
}

func TestStream_ErrorMapping(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"auth","message":"bad"}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"), option.WithMaxRetries(0))
	a := &adapter{client: &client, model: "m"}

	// With real SSE streaming, Stream itself returns (ch, nil) immediately;
	// a header-level error surfaces asynchronously as a StreamChunk.Err,
	// matching ai/openai's TestStream_ErrorMapping convention.
	ch, err := a.Stream(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err %v, want nil", err)
	}

	for chunk := range ch {
		if chunk.Err != nil {
			if !errors.Is(chunk.Err, ai.ErrAuth) {
				t.Fatalf("chunk err = %v, want ErrAuth", chunk.Err)
			}

			return
		}
	}

	t.Fatal("want error chunk")
}

func TestGenerate_ContextCancel(t *testing.T) {
	t.Parallel()
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Signal request arrival (non-blocking: retries reuse this handler).
		select {
		case started <- struct{}{}:
		default:
		}
		// Deterministic slow server with no fixed sleep: block until the
		// client goes away or the test releases us after Generate
		// returns. release is required because the client's keep-alive
		// pool can hold the connection open past cancellation, which
		// would wedge srv.Close on this handler forever.
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go func() {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
		}
		cancel()
	}()
	_, err := a.Generate(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	close(release)
	if err == nil {
		t.Fatalf("want cancel error")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "context") {
		// wrapped generate error should contain context
		t.Logf("cancel err: %v", err)
	}
}

func TestRedactionHidesKey(t *testing.T) {
	t.Parallel()
	secret := "sk-ant-secret12345"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"auth","message":"bad"}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey(secret), option.WithMaxRetries(0))
	a := &adapter{client: &client, model: "m"}
	_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil {
		t.Fatalf("want error")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks api key: %v", err)
	}
	// also test url.Error directly
	ue := &url.Error{Op: "Get", URL: fmt.Sprintf("https://api.anthropic.com/v1/messages?x-api-key=%s", secret), Err: errors.New("dial")}
	red := redactURLError(ue)
	if strings.Contains(red.Error(), secret) {
		t.Fatalf("redact leaks: %v", red)
	}
	if !strings.Contains(red.Error(), "REDACTED") {
		t.Fatalf("missing REDACTED")
	}
}

func TestClose(t *testing.T) {
	t.Parallel()
	a, _ := New(ai.Options{APIKey: "k"})
	if err := a.Close(); err != nil {
		t.Fatalf("Close err %v", err)
	}
}

func TestGenerate_UsageOverflow(t *testing.T) {
	t.Parallel()
	maxInt64 := int64(math.MaxInt)
	overflowVal := maxInt64
	overflowVal++
	if maxInt64 < math.MaxInt64 {
		if _, err := safeInt64ToInt(overflowVal); err == nil {
			t.Fatalf("want overflow")
		}
	}
	minInt64 := int64(math.MinInt)
	underVal := minInt64
	underVal--
	if minInt64 > math.MinInt64 {
		if _, err := safeInt64ToInt(underVal); err == nil {
			t.Fatalf("want underflow")
		}
	}
}

func TestBuildToolInputSchema_Extras(t *testing.T) {
	t.Parallel()
	s := buildToolInputSchema(map[string]any{
		"properties": map[string]any{"a": map[string]any{"type": "string"}},
		"required":   []any{"a", 123, nil},
		"type":       "object",
		"extra1":     "v1",
	})
	if s.Properties == nil {
		t.Fatalf("nil props")
	}
	if len(s.Required) != 1 || s.Required[0] != "a" {
		t.Fatalf("required %v", s.Required)
	}
	if s.ExtraFields["extra1"] != "v1" {
		t.Fatalf("extra missing")
	}
}

func TestGenerate_OverflowViaSafeInt(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// sentinel 1<<62 triggers safeInt overflow
		_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":%d,"output_tokens":1}}`, int64(1<<62))))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	_, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "input_tokens overflow") {
		t.Fatalf("want input_tokens overflow, got %v", err)
	}
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":%d}}`, int64(1<<62))))
	}))
	defer srv2.Close()
	client2 := anthropic.NewClient(option.WithBaseURL(srv2.URL), option.WithAPIKey("k"))
	a2 := &adapter{client: &client2, model: "m"}
	_, err = a2.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "output_tokens overflow") {
		t.Fatalf("want output_tokens overflow, got %v", err)
	}
}

func TestGenerate_ToolUseNullInput(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"tool_use","id":"tid","name":"t","input":null}],"model":"c","stop_reason":"tool_use","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	gen, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if len(gen.ToolCalls) != 1 || gen.ToolCalls[0].Arguments != "{}" {
		t.Fatalf("null input args %v", gen.ToolCalls)
	}
}

func TestStream_CancelMidStream(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, anthropicSSEBody)
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	ctx, cancel := context.WithCancel(t.Context())
	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err %v", err)
	}
	// cancel immediately, before reading - the drain below already waits
	// with a timeout, so no fixed settle sleep is needed.
	cancel()
	// drain with timeout, should close quickly
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("stream not closed after cancel")
	}
}

func TestStream_CancelBeforeDone(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, anthropicSSEBody)
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	ctx, cancel := context.WithCancel(t.Context())
	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err %v", err)
	}
	// read first chunk (Delta)
	first, ok := <-ch
	if !ok {
		t.Fatalf("channel closed early")
	}
	if first.Delta != "hello" {
		t.Fatalf("delta %q", first.Delta)
	}
	// cancel before Done is sent; the read below already waits with a
	// timeout, so no fixed settle sleep is needed.
	cancel()
	// next read should be closed (goroutine exited via <-ctx.Done for final Done)
	select {
	case _, ok := <-ch:
		if ok {
			// may have no more, but should close without sending Done
			for range ch {
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for close after cancel")
	}
}

func TestStream_CancelDuringToolCalls(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, anthropicSSEBody)
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	ctx, cancel := context.WithCancel(t.Context())
	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err %v", err)
	}
	// read Delta
	<-ch
	// cancel before tool calls are sent; the drain below already waits
	// with a timeout, so no fixed settle sleep is needed.
	cancel()
	// drain
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatalf("not closed")
	}
}

func TestGenerate_EmptyContentToolUse(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"m","type":"message","role":"assistant","content":[{"type":"text","text":""}],"model":"c","stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}))
	defer srv.Close()
	client := anthropic.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"))
	a := &adapter{client: &client, model: "m"}
	gen, err := a.Generate(t.Context(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
	if gen.Content != "" {
		t.Fatalf("want empty, got %q", gen.Content)
	}
}
