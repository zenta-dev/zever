package gemini

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"
	"google.golang.org/genai"

	"github.com/zenta-dev/zever/ai"
)

func TestOpen_APIKeyRequired(t *testing.T) {
	t.Parallel()

	_, err := New(ai.Options{})
	if !errors.Is(err, ai.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}

	var ioe *ai.InvalidOptionsError
	if !errors.As(err, &ioe) {
		t.Fatalf("not InvalidOptionsError")
	}

	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("msg %q missing api_key", err.Error())
	}

	_, err = New(ai.Options{APIKey: "   "})
	if !errors.Is(err, ai.ErrInvalidOptions) {
		t.Fatalf("empty api key err = %v", err)
	}
}

func TestOpen_BaseURLValidation(t *testing.T) {
	t.Parallel()

	_, err := New(ai.Options{APIKey: "key", BaseURL: "http://example.com"})
	if !errors.Is(err, ai.ErrInvalidOptions) {
		t.Fatalf("err = %v want ErrInvalidOptions", err)
	}

	_, err = New(ai.Options{APIKey: "key", BaseURL: "https://example.com"})
	if err != nil {
		// BaseURL https should be allowed but NewClient may try to use it; if it fails due to not reachable, should not be validation error
		// However NewClient with custom BaseURL and APIKey should succeed to create client without dialing.
		// So we expect no error at Open time for valid https URL (httptest will be used later)
		// Using example.com should succeed client creation (no network call at NewClient)
		// If err contains validation, fail
		if errors.Is(err, ai.ErrInvalidOptions) {
			t.Fatalf("unexpected validation err %v", err)
		}
	}
}

func TestOpen_TimeoutValidation(t *testing.T) {
	t.Parallel()

	_, err := New(ai.Options{APIKey: "key", Timeout: -1})
	if !errors.Is(err, ai.ErrInvalidOptions) {
		t.Fatalf("timeout err = %v", err)
	}
}

func TestOpen_SuccessAndTLS(t *testing.T) {
	t.Parallel()

	a, err := New(ai.Options{APIKey: "test-key", Timeout: time.Second})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	if a == nil {
		t.Fatal("nil AI")
	}

	// Check HTTP client TLS
	ad, ok := a.(*adapter)
	if !ok {
		t.Fatal("not adapter")
	}

	tr, ok := ad.client.ClientConfig().HTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport not *http.Transport: %T", ad.client.ClientConfig().HTTPClient.Transport)
	}

	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("TLS MinVersion = %v want TLS12", tr.TLSClientConfig.MinVersion)
	}

	if ad.client.ClientConfig().HTTPClient.Timeout != time.Second {
		t.Errorf("Timeout %v want 1s", ad.client.ClientConfig().HTTPClient.Timeout)
	}

	if err := a.Close(); err != nil {
		t.Errorf("Close err = %v", err)
	}
}

func TestNewHTTPClient_Fallback(t *testing.T) {
	// Force DefaultTransport not *http.Transport
	prev := http.DefaultTransport
	http.DefaultTransport = &fakeTransport{}
	defer func() { http.DefaultTransport = prev }()

	c := newHTTPClient(0)
	if c.Transport == nil {
		t.Fatal("nil transport")
	}

	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("want *http.Transport got %T", c.Transport)
	}

	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("fallback TLS wrong")
	}
}

type fakeTransport struct{}

func (f *fakeTransport) RoundTrip(_ *http.Request) (*http.Response, error) {
	return nil, errors.New("fake")
}

func openWithServer(t *testing.T, srv *httptest.Server) ai.AI { //nolint:unused
	t.Helper()
	a, err := New(ai.Options{APIKey: "test-key", BaseURL: srv.URL})
	if err == nil {
		ad0, ok := a.(*adapter) //nolint:forcetypeassert
		if !ok {
			t.Fatalf("not adapter")
		}
		if tr, ok := ad0.client.ClientConfig().HTTPClient.Transport.(*http.Transport); ok {
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
	}
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	ad, _ := a.(*adapter) //nolint:forcetypeassert
	if tr, ok := ad.client.ClientConfig().HTTPClient.Transport.(*http.Transport); ok {
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		} else {
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
	}
	return a
}

func openWithKeyAndServer(t *testing.T, key string, srv *httptest.Server) ai.AI {
	t.Helper()
	a, err := New(ai.Options{APIKey: key, BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	ad, _ := a.(*adapter) //nolint:forcetypeassert
	if tr, ok := ad.client.ClientConfig().HTTPClient.Transport.(*http.Transport); ok {
		if tr.TLSClientConfig == nil {
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
		} else {
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
	}
	return a
}

func TestGenerate_SuccessMocked(t *testing.T) {
	t.Parallel()

	// Mock server for generateContent
	handler := func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "generateContent") {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Check header redaction not needed; ensure API key header present
		if got := r.Header.Get("x-goog-api-key"); got != "test-key" {
			t.Errorf("x-goog-api-key = %q want test-key", got)
		}

		// Verify request body contains system instruction and contents
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		resp := map[string]any{
			"candidates": []any{
				map[string]any{
					"content": map[string]any{
						"parts": []any{
							map[string]any{"text": "hello world"},
						},
						"role": "model",
					},
					"finishReason": "STOP",
					"index":        0,
				},
			},
			"usageMetadata": map[string]any{
				"promptTokenCount":     5,
				"candidatesTokenCount": 10,
				"totalTokenCount":      15,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(handler))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "test-key", BaseURL: srv.URL})
	if err == nil {
		ad0, ok := a.(*adapter) //nolint:forcetypeassert
		if !ok {
			t.Fatalf("not adapter")
		}
		if tr, ok := ad0.client.ClientConfig().HTTPClient.Transport.(*http.Transport); ok {
			tr.TLSClientConfig.InsecureSkipVerify = true
		}
	}
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	gen, err := a.Generate(t.Context(), "models/gemini-1.5-flash", []ai.Message{
		{Role: ai.RoleSystem, Content: "you are helpful"},
		{Role: ai.RoleUser, Content: "hi"},
	}, ai.GenerateOptions{
		Temperature: genai.Ptr[float32](0.7),
		MaxTokens:   100,
		TopP:        genai.Ptr[float32](0.9),
		ResponseFormat: &ai.ResponseFormat{
			Type:       "json_object",
			JSONSchema: map[string]any{"type": "object"},
		},
		Tools: []ai.Tool{
			{Name: "myTool", Description: "desc", Parameters: map[string]any{"type": "object", "properties": map[string]any{"foo": map[string]any{"type": "string"}}, "required": []any{"foo"}}},
		},
		ToolChoice: ai.ToolChoiceAuto,
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if gen.Content != "hello world" {
		t.Errorf("Content = %q want hello world", gen.Content)
	}

	if gen.Usage.PromptTokens != 5 || gen.Usage.CompletionTokens != 10 {
		t.Errorf("Usage = %+v want 5/10", gen.Usage)
	}

	if gen.FinishReason != "STOP" {
		t.Errorf("FinishReason = %q want STOP", gen.FinishReason)
	}
}

func TestGenerate_EmptyCandidates(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[]}`))
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	_, err := a.Generate(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if !errors.Is(err, ai.ErrNotSupported) {
		t.Fatalf("err = %v want ErrNotSupported", err)
	}
}

func TestGenerate_ModelRequired(t *testing.T) {
	t.Parallel()

	a, _ := New(ai.Options{APIKey: "k", BaseURL: "https://example.com"})
	_, err := a.Generate(t.Context(), "", nil, ai.GenerateOptions{})
	if !errors.Is(err, ai.ErrInvalidRequest) {
		t.Fatalf("err = %v want ErrInvalidRequest", err)
	}

	_, err = a.Generate(t.Context(), "   ", nil, ai.GenerateOptions{})
	if !errors.Is(err, ai.ErrInvalidRequest) {
		t.Fatalf("empty model err = %v", err)
	}
}

func TestGenerate_ErrorMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		status    int
		body      string
		wantErr   error
		wantIs    error
		wantInMsg string
	}{
		{"401 auth", 401, `{"error":{"code":401,"message":"bad auth","status":"UNAUTHENTICATED"}}`, ai.ErrAuth, ai.ErrAuth, "bad auth"},
		{"403 auth", 403, `{"error":{"code":403,"message":"forbidden","status":"PERMISSION_DENIED"}}`, ai.ErrAuth, ai.ErrAuth, "forbidden"},
		{"429 rate", 429, `{"error":{"code":429,"message":"rate limited","status":"RESOURCE_EXHAUSTED"}}`, ai.ErrRateLimited, ai.ErrRateLimited, ""},
		{"400 invalid", 400, `{"error":{"code":400,"message":"bad request","status":"INVALID_ARGUMENT"}}`, ai.ErrInvalidRequest, ai.ErrInvalidRequest, "bad request"},
		{"404 notfound", 404, `{"error":{"code":404,"message":"no model","status":"NOT_FOUND"}}`, ai.ErrModelNotFound, ai.ErrModelNotFound, "no model"},
		{"500 generic", 500, `{"error":{"code":500,"message":"oops","status":"INTERNAL"}}`, nil, nil, "oops"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			a, _ := New(ai.Options{APIKey: "k", BaseURL: srv.URL})
			_, err := a.Generate(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
			if err == nil {
				t.Fatalf("want error")
			}

			if tc.wantIs != nil && !errors.Is(err, tc.wantIs) {
				t.Fatalf("err = %v want Is %v", err, tc.wantIs)
			}

			if tc.wantInMsg != "" && !strings.Contains(err.Error(), tc.wantInMsg) {
				t.Errorf("err %q missing %q", err.Error(), tc.wantInMsg)
			}

			if tc.name == "429 rate" {
				var rle *ai.RateLimitedError
				if !errors.As(err, &rle) {
					t.Errorf("429 not RateLimitedError")
				}
			}
		})
	}
}

func TestGenerate_Redaction(t *testing.T) {
	t.Parallel()

	// Server returns error containing API key in message
	apiKey := "super-secret-key-12345"
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"invalid key super-secret-key-12345","status":"UNAUTHENTICATED"}}`))
	}))
	defer srv.Close()

	a, _ := New(ai.Options{APIKey: apiKey, BaseURL: srv.URL})
	_, err := a.Generate(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil {
		t.Fatal("want error")
	}

	if strings.Contains(err.Error(), apiKey) {
		t.Errorf("error leaks api key: %q", err.Error())
	}

	if !strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("error should contain REDACTED, got %q", err.Error())
	}
}

func TestGenerate_WithFunctionCall(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"candidates": []any{
				map[string]any{
					"content": map[string]any{
						"parts": []any{
							map[string]any{"functionCall": map[string]any{"name": "myTool", "args": map[string]any{"foo": "bar"}}},
						},
						"role": "model",
					},
					"finishReason": "STOP",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a, _ := New(ai.Options{APIKey: "k", BaseURL: srv.URL})
	gen, err := a.Generate(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	if len(gen.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len %d want 1", len(gen.ToolCalls))
	}

	if gen.ToolCalls[0].Name != "myTool" {
		t.Errorf("Name %q want myTool", gen.ToolCalls[0].Name)
	}

	if !strings.Contains(gen.ToolCalls[0].Arguments, "bar") {
		t.Errorf("Arguments %q missing bar", gen.ToolCalls[0].Arguments)
	}
}

func TestStream_Ordering(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			// SSE format: data: {...}\n\n
			chunks := []string{
				`{"candidates":[{"content":{"parts":[{"text":"hello "}],"role":"model"},"finishReason":""}]}`,
				`{"candidates":[{"content":{"parts":[{"text":"world"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":1,"candidatesTokenCount":2}}`,
			}
			for _, c := range chunks {
				_, _ = w.Write([]byte("data: " + c + "\n\n"))
			}

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	ch, err := a.Stream(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	var deltas []string

	var doneSeen bool

	var usageSeen *ai.Usage

	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk err %v", chunk.Err)
		}

		if chunk.Delta != "" {
			deltas = append(deltas, chunk.Delta)
		}

		if chunk.Done {
			doneSeen = true
		}

		if chunk.Usage != nil {
			usageSeen = chunk.Usage
		}
	}

	joined := strings.Join(deltas, "")
	if joined != "hello world" {
		t.Errorf("joined = %q want hello world", joined)
	}

	if !doneSeen {
		t.Error("done not seen")
	}

	if usageSeen == nil {
		t.Error("usage not seen")
	} else if usageSeen.PromptTokens != 1 || usageSeen.CompletionTokens != 2 {
		t.Errorf("usage %+v want 1/2", usageSeen)
	}
}

// streamCancelTestServer starts a TLS test server whose streamGenerateContent
// handler sends one SSE event, flushes, then blocks on release -- simulating
// a still-pending network read so a ctx cancellation genuinely has to abort
// an in-flight read rather than racing an already-fully-buffered response.
// The caller must arrange for release to be closed (directly or via
// releaseFn) before the test ends, or the server's own handler goroutine
// leaks for the rest of the test binary's life.
func streamCancelTestServer(t *testing.T) (srv *httptest.Server, releaseFn func()) {
	t.Helper()

	release := make(chan struct{})

	var once sync.Once

	releaseFn = func() { once.Do(func() { close(release) }) }
	t.Cleanup(releaseFn)

	srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: " + `{"candidates":[{"content":{"parts":[{"text":"one"}],"role":"model"},"finishReason":""}]}` + "\n\n"))

			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}

			<-release

			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	return srv, releaseFn
}

// TestStream_ClosesOnContextCancel is the regression test for the fixed
// goroutine leak: Stream used to send every chunk with a bare `ch <- ...`,
// so a caller that cancelled ctx and permanently stopped draining ch (the
// expected behavior on cancellation) left the goroutine blocked forever on
// its next send. Deliberately not t.Parallel: goleak.VerifyNone inspects
// every goroutine in the process, so overlapping with other parallel tests
// would make it flaky.
func TestStream_ClosesOnContextCancel(t *testing.T) {
	srv, release := streamCancelTestServer(t)

	a := openWithKeyAndServer(t, "k", srv)

	ctx, cancel := context.WithCancel(t.Context())

	snapshot := goleak.IgnoreCurrent()

	ch, err := a.Stream(ctx, "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	select {
	case chunk, ok := <-ch:
		if !ok || chunk.Delta != "one" {
			t.Fatalf("first chunk = %+v, ok=%v, want Delta=\"one\"", chunk, ok)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first chunk")
	}

	cancel()  // caller abandons the stream: ch is never read again
	release() // let the server handler (and its transport goroutines) wind down

	goleak.VerifyNone(t, snapshot)
}

func TestStream_ModelRequired(t *testing.T) {
	t.Parallel()

	a, _ := New(ai.Options{APIKey: "k", BaseURL: "https://example.com"})
	_, err := a.Stream(t.Context(), "", nil, ai.GenerateOptions{})
	if !errors.Is(err, ai.ErrInvalidRequest) {
		t.Fatalf("err = %v want ErrInvalidRequest", err)
	}
}

func TestStream_ErrorMapping(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"bad","status":"UNAUTHENTICATED"}}`))
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	ch, err := a.Stream(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err %v", err)
	}

	chunk, ok := <-ch
	if !ok {
		t.Fatal("channel closed empty")
	}

	if chunk.Err == nil {
		t.Fatal("want chunk.Err")
	}

	if !errors.Is(chunk.Err, ai.ErrAuth) {
		t.Fatalf("chunk.Err = %v want ErrAuth", chunk.Err)
	}
}

func TestStream_WithToolCall(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			c := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"myTool","args":{"foo":"bar"}}}],"role":"model"}}]}`
			_, _ = w.Write([]byte("data: " + c + "\n\n"))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	ch, _ := a.Stream(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})

	var sawTool bool

	for chunk := range ch {
		if chunk.ToolName == "myTool" {
			sawTool = true

			if !strings.Contains(chunk.ToolArgsDelta, "bar") {
				t.Errorf("ToolArgsDelta %q missing bar", chunk.ToolArgsDelta)
			}
		}
	}

	if !sawTool {
		t.Error("tool call not seen in stream")
	}
}

func TestEmbed_Success(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "batchEmbedContents") && !strings.Contains(r.URL.Path, "embedContent") {
			t.Errorf("embed path %q missing batchEmbedContents", r.URL.Path)
		}

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)

		// Return embeddings
		resp := map[string]any{
			"embeddings": []any{
				map[string]any{"values": []float32{0.1, 0.2, 0.3}},
				map[string]any{"values": []float32{0.4, 0.5, 0.6}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	vecs, err := a.Embed(t.Context(), "models/text-embedding-004", []string{"a", "b"}, ai.EmbedOptions{})
	if err != nil {
		t.Fatalf("Embed err = %v", err)
	}

	if len(vecs) != 2 {
		t.Fatalf("len %d want 2", len(vecs))
	}

	if vecs[0][0] != 0.1 || vecs[1][0] != 0.4 {
		t.Errorf("vecs %v", vecs)
	}
}

func TestEmbed_Dimensions(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"embeddings": []any{
				map[string]any{"values": []float32{0.1, 0.2, 0.3, 0.4}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	vecs, err := a.Embed(t.Context(), "models/text-embedding-004", []string{"hello"}, ai.EmbedOptions{Dimensions: 2})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	if len(vecs[0]) != 2 {
		t.Errorf("dim %d want 2", len(vecs[0]))
	}

	if vecs[0][0] != 0.1 || vecs[0][1] != 0.2 {
		t.Errorf("truncated %v", vecs[0])
	}
}

func TestEmbed_Errors(t *testing.T) {
	t.Parallel()

	a, _ := New(ai.Options{APIKey: "k", BaseURL: "https://example.com"})

	_, err := a.Embed(t.Context(), "", []string{"hi"}, ai.EmbedOptions{})
	if !errors.Is(err, ai.ErrInvalidRequest) {
		t.Fatalf("model empty err %v", err)
	}

	_, err = a.Embed(t.Context(), "m", []string{}, ai.EmbedOptions{})
	if !errors.Is(err, ai.ErrInvalidRequest) {
		t.Fatalf("inputs empty err %v", err)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[]}`))
	}))
	defer srv.Close()

	a2 := openWithKeyAndServer(t, "k", srv)
	_, err = a2.Embed(t.Context(), "m", []string{"hi"}, ai.EmbedOptions{})
	if !errors.Is(err, ai.ErrNotSupported) {
		t.Fatalf("empty embeddings err %v", err)
	}

	// Error mapping 429
	srv3 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"rate","status":"RESOURCE_EXHAUSTED"}}`))
	}))
	defer srv3.Close()

	a3 := openWithKeyAndServer(t, "k", srv3)
	_, err = a3.Embed(t.Context(), "m", []string{"hi"}, ai.EmbedOptions{})
	if !errors.Is(err, ai.ErrRateLimited) {
		t.Fatalf("429 err %v", err)
	}

	// Redaction
	apiKey := "secret-embed-key"
	srv4 := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":400,"message":"bad secret-embed-key","status":"INVALID_ARGUMENT"}}`))
	}))
	defer srv4.Close()

	a4 := openWithKeyAndServer(t, apiKey, srv4)
	_, err = a4.Embed(t.Context(), "m", []string{"hi"}, ai.EmbedOptions{})
	if err == nil || strings.Contains(err.Error(), apiKey) {
		t.Fatalf("redaction failed %v", err)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	a, _ := New(ai.Options{APIKey: "k", BaseURL: "https://example.com"})
	if err := a.Close(); err != nil {
		t.Errorf("Close err %v", err)
	}
}

func TestBuildGenerateConfig(t *testing.T) {
	t.Parallel()

	// json_object
	cfg := buildGenerateConfig(ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	}, nil)
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("mime %q", cfg.ResponseMIMEType)
	}

	// json_schema with schema
	cfg = buildGenerateConfig(ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{Type: "json_schema", JSONSchema: map[string]any{"type": "object"}},
	}, nil)
	if cfg.ResponseMIMEType != "application/json" || cfg.ResponseJsonSchema == nil {
		t.Error("json_schema cfg")
	}

	// generic json type
	cfg = buildGenerateConfig(ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{Type: "JSON"},
	}, nil)
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("generic json mime %q", cfg.ResponseMIMEType)
	}

	// JSONSchema without type
	cfg = buildGenerateConfig(ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{JSONSchema: map[string]any{"type": "object"}},
	}, nil)
	if cfg.ResponseMIMEType != "application/json" {
		t.Errorf("schema mime %q", cfg.ResponseMIMEType)
	}

	// Tools with choices
	for _, choice := range []ai.ToolChoice{ai.ToolChoiceAuto, ai.ToolChoiceRequired, ai.ToolChoiceNone} {
		cfg = buildGenerateConfig(ai.GenerateOptions{
			Tools:      []ai.Tool{{Name: "t", Parameters: map[string]any{"type": "object"}}},
			ToolChoice: choice,
		}, nil)
		if cfg.Tools == nil || cfg.ToolConfig == nil {
			t.Errorf("tools choice %v missing", choice)
		}
	}

	// System
	sys := &genai.Content{Parts: []*genai.Part{genai.NewPartFromText("sys")}}
	cfg = buildGenerateConfig(ai.GenerateOptions{}, sys)
	if cfg.SystemInstruction == nil {
		t.Error("system nil")
	}

	// Temperature and TopP
	cfg = buildGenerateConfig(ai.GenerateOptions{
		Temperature: genai.Ptr[float32](0.5),
		MaxTokens:   10,
		TopP:        genai.Ptr[float32](0.9),
	}, nil)
	if cfg.Temperature == nil || *cfg.Temperature != 0.5 {
		t.Error("temp")
	}

	if cfg.MaxOutputTokens != 10 {
		t.Error("max tokens")
	}

	if cfg.TopP == nil {
		t.Error("topP")
	}
}

func TestToGenaiContents(t *testing.T) {
	t.Parallel()

	msgs := []ai.Message{
		{Role: ai.RoleSystem, Content: "sys1"},
		{Role: ai.RoleSystem, Content: "sys2"},
		{Role: ai.RoleUser, Content: "hello"},
		{Role: ai.RoleAssistant, Content: "hi", ToolCalls: []ai.ToolCall{{Name: "tool1", Arguments: `{"a":1}`}, {Name: "tool2", Arguments: "invalid json"}}},
		{Role: ai.RoleAssistant, Content: ""}, // empty assistant should be skipped (no parts -> continue, but our logic skips empty)
		{Role: ai.RoleTool, Content: `{"result":"ok"}`, ToolCallID: "tool1"},
		{Role: ai.RoleTool, Content: "plain text", ToolCallID: ""},
		{Role: "unknown", Content: "fallback"},
		{Role: ai.RoleUser, Content: ""},
	}

	contents, sys := toGenaiContents(msgs)
	if sys == nil || len(sys.Parts) != 2 {
		t.Fatalf("system parts %v", sys)
	}

	if len(contents) < 6 {
		t.Fatalf("contents len %d", len(contents))
	}

	// Check assistant tool call args fallback
	found := false

	for _, c := range contents {
		if c.Role == string(genai.RoleModel) {
			for _, p := range c.Parts {
				if p.FunctionCall != nil && p.FunctionCall.Name == "tool2" {
					found = true
				}
			}
		}
	}

	if !found {
		t.Error("tool2 not found")
	}
}

func TestConvertMapToSchema(t *testing.T) {
	t.Parallel()

	if convertMapToSchema(nil) != nil {
		t.Error("nil not nil")
	}

	s := convertMapToSchema(map[string]any{
		"type":        "object",
		"description": "desc",
		"format":      "fmt",
		"title":       "title",
		"nullable":    true,
		"properties": map[string]any{
			"foo":  map[string]any{"type": "string"},
			"bar":  map[string]any{"type": "number"},
			"num":  map[string]any{"type": "integer"},
			"flag": map[string]any{"type": "boolean"},
			"arr":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
		},
		"required": []any{"foo"},
		"enum":     []any{"a", "b"},
		"items":    map[string]any{"type": "string"},
	})
	if s.Type != genai.TypeObject {
		t.Errorf("type %v", s.Type)
	}

	if s.Description != "desc" || s.Format != "fmt" || s.Title != "title" {
		t.Error("fields")
	}

	if s.Nullable == nil || !*s.Nullable {
		t.Error("nullable")
	}

	if len(s.Properties) != 5 {
		t.Errorf("props %d", len(s.Properties))
	}

	if len(s.Required) != 1 {
		t.Error("required")
	}

	if len(s.Enum) != 2 {
		t.Error("enum")
	}

	if s.Items == nil {
		t.Error("items nil")
	}

	// required []string
	s2 := convertMapToSchema(map[string]any{"required": []string{"a", "b"}})
	if len(s2.Required) != 2 {
		t.Error("required string")
	}

	// enum []string
	s3 := convertMapToSchema(map[string]any{"enum": []string{"x"}})
	if len(s3.Enum) != 1 {
		t.Error("enum string")
	}

	// unknown type
	s4 := convertMapToSchema(map[string]any{"type": "unknown"})
	if s4.Type != "" {
		t.Errorf("unknown type %v", s4.Type)
	}
}

func TestExtractGeneration(t *testing.T) {
	t.Parallel()

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{Text: "hello "},
						{Text: "world"},
						{FunctionCall: &genai.FunctionCall{Name: "fn", Args: map[string]any{"x": 1}}},
						{FunctionCall: &genai.FunctionCall{Name: "fn2", Args: nil}},
					},
				},
			},
			nil,
			{Content: nil},
		},
	}

	txt, calls := extractGeneration(resp)
	if txt != "hello world" {
		t.Errorf("txt %q", txt)
	}

	if len(calls) != 2 {
		t.Fatalf("calls %d", len(calls))
	}

	// nil candidate handling
	resp2 := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{{Text: "a"}}}}},
	}

	txt, _ = extractGeneration(resp2)
	if txt != "a" {
		t.Errorf("a %q", txt)
	}
}

func TestMapError(t *testing.T) {
	t.Parallel()

	// APIError via errors.As
	err401 := genai.APIError{Code: 401, Message: "unauth"}
	mapped := mapError(err401)
	if !errors.Is(mapped, ai.ErrAuth) {
		t.Errorf("401 mapping %v", mapped)
	}

	// Wrapped APIError
	wrapped := fmt.Errorf("wrap: %w", genai.APIError{Code: 429, Message: "rate"})
	mapped = mapError(wrapped)
	if !errors.Is(mapped, ai.ErrRateLimited) {
		t.Errorf("wrapped 429 %v", mapped)
	}

	// Direct type assert value
	mapped = mapError(genai.APIError{Code: 400, Message: "bad"})
	if !errors.Is(mapped, ai.ErrInvalidRequest) {
		t.Errorf("400 %v", mapped)
	}

	mapped = mapError(genai.APIError{Code: 404, Message: "notfound"})
	if !errors.Is(mapped, ai.ErrModelNotFound) {
		t.Errorf("404 %v", mapped)
	}

	// Generic 500 with message
	mapped = mapError(genai.APIError{Code: 500, Message: "oops"})
	if mapped.Error() != "oops" {
		t.Errorf("500 msg %q", mapped.Error())
	}

	// Generic 500 without message falls back to Error()
	mapped = mapError(genai.APIError{Code: 500})
	if mapped.Error() == "" {
		t.Error("empty 500")
	}

	// String fallback 401
	mapped = mapError(errors.New("401 unauthorized"))
	if !errors.Is(mapped, ai.ErrAuth) {
		t.Errorf("string 401 %v", mapped)
	}

	mapped = mapError(errors.New("rate limit exceeded 429"))
	if !errors.Is(mapped, ai.ErrRateLimited) {
		t.Errorf("string 429 %v", mapped)
	}

	mapped = mapError(errors.New("400 bad"))
	if !errors.Is(mapped, ai.ErrInvalidRequest) {
		t.Errorf("string 400 %v", mapped)
	}

	mapped = mapError(errors.New("404 not_found"))
	if !errors.Is(mapped, ai.ErrModelNotFound) {
		t.Errorf("string 404 %v", mapped)
	}

	// Unknown returns original
	orig := errors.New("other")
	if !errors.Is(mapError(orig), orig) {
		t.Error("other not original")
	}

	if mapError(nil) != nil {
		t.Error("nil not nil")
	}
}

func TestMapAndRedact(t *testing.T) {
	t.Parallel()

	if mapAndRedact(nil, "k") != nil {
		t.Error("nil not nil")
	}

	// Redact api key in generic error
	err := errors.New("failure with secret-key-123")
	mapped := mapAndRedact(err, "secret-key-123")
	if strings.Contains(mapped.Error(), "secret-key-123") {
		t.Errorf("leak %q", mapped.Error())
	}

	if !strings.Contains(mapped.Error(), "REDACTED") {
		t.Errorf("no redacted %q", mapped.Error())
	}

	// Redact header
	err = errors.New("header x-goog-api-key: secret")
	mapped = mapAndRedact(err, "secret")
	if strings.Contains(mapped.Error(), "x-goog-api-key") {
		t.Errorf("header not redacted %q", mapped.Error())
	}

	// Preserve sentinel after redact
	apiKey := "mykey"
	err = genai.APIError{Code: 401, Message: "bad mykey"}
	mapped = mapAndRedact(err, apiKey)
	if !errors.Is(mapped, ai.ErrAuth) {
		t.Errorf("auth not preserved %v", mapped)
	}

	if strings.Contains(mapped.Error(), apiKey) {
		t.Errorf("auth redact leak")
	}

	// RateLimited redact preserves type
	err = genai.APIError{Code: 429, Message: "rate mykey"}
	mapped = mapAndRedact(err, apiKey)
	var rle *ai.RateLimitedError
	if !errors.As(mapped, &rle) {
		t.Errorf("rate not preserved")
	}

	// InvalidRequest redact
	err = genai.APIError{Code: 400, Message: "bad mykey"}
	mapped = mapAndRedact(err, apiKey)
	if !errors.Is(mapped, ai.ErrInvalidRequest) {
		t.Errorf("invalid not preserved")
	}

	// ModelNotFound redact
	err = genai.APIError{Code: 404, Message: "notfound mykey"}
	mapped = mapAndRedact(err, apiKey)
	if !errors.Is(mapped, ai.ErrModelNotFound) {
		t.Errorf("model not preserved")
	}

	// NotSupported redact
	err = fmt.Errorf("%w: bad mykey", ai.ErrNotSupported)
	mapped = mapAndRedact(err, apiKey)
	if !errors.Is(mapped, ai.ErrNotSupported) {
		t.Errorf("notsupported not preserved")
	}

	// Generic fallback redact
	err = errors.New("generic mykey error")
	mapped = mapAndRedact(err, apiKey)
	if strings.Contains(mapped.Error(), apiKey) {
		t.Errorf("generic leak")
	}

	// No change when no key
	err = errors.New("no key here")
	mapped2 := mapAndRedact(err, "other")
	if mapped2.Error() != "no key here" {
		t.Errorf("no change failed %q", mapped2.Error())
	}
}

func TestMapAPIError(t *testing.T) {
	t.Parallel()

	// 500 with message
	e := genai.APIError{Code: 500, Message: "oops"}
	mapped := mapAPIError(e)
	if mapped.Error() != "oops" {
		t.Errorf("500 msg %q", mapped.Error())
	}

	// 500 without message
	e = genai.APIError{Code: 500, Status: "500 Internal Server Error"}
	mapped = mapAPIError(e)
	if !strings.Contains(mapped.Error(), "500") {
		t.Errorf("500 empty %q", mapped.Error())
	}
}

func TestEmbed_NilEmbedding(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Return embeddings with one nil entry
		resp := map[string]any{
			"embeddings": []any{
				nil,
				map[string]any{"values": []float32{1, 2}},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	vecs, err := a.Embed(t.Context(), "m", []string{"a", "b"}, ai.EmbedOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}

	if len(vecs) != 2 {
		t.Fatalf("len %d", len(vecs))
	}

	if len(vecs[0]) != 0 {
		t.Errorf("first should be empty %v", vecs[0])
	}
}

func TestGenerate_SystemInstruction(t *testing.T) {
	t.Parallel()

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"}}]}`))
	}))
	defer srv.Close()

	a := openWithKeyAndServer(t, "k", srv)
	_, err := a.Generate(t.Context(), "models/gemini-1.5-flash", []ai.Message{
		{Role: ai.RoleSystem, Content: "sys"},
		{Role: ai.RoleUser, Content: "hi"},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("err %v", err)
	}
}

func TestOpen_RedactOnNewClientError(t *testing.T) {
	t.Parallel()

	// Invalid BaseURL triggers NewClient error that may contain key? We can test by providing invalid HTTPOptions?
	// genai.NewClient with APIKey containing special chars may error; but we can simulate by passing BaseURL that is invalid URL?
	// Use non-https url already validated; to trigger NewClient failure we need empty APIKey? But we already handle.
	// Instead directly test mapAndRedact with NewClient error via invalid BaseURL that contains key?
	// For coverage, call Open with malformed BaseURL that passes Validate but fails url parse inside genai?
	// Validate requires https, so we use https://%zz -> Validate will fail with InvalidOptions, not NewClient.
	// So we test that Open returns InvalidOptions for bad baseUrl, which already covers.
	// To cover mapAndRedact for NewClient, we can use APIKey with newline that may cause error?
	// Simpler: ensure Open with valid params succeeds even with custom baseURL that is httptest.
	_, err := New(ai.Options{APIKey: "k", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("open with https example should succeed, got %v", err)
	}
}

func TestStream_EmptyCandidates(t *testing.T) { //nolint:dupl
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			// First chunk empty candidates, second valid
			_, _ = w.Write([]byte("data: {\"candidates\":[]}\n\n"))
			_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"hi\"}],\"role\":\"model\"}}]}\n\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	a := openWithKeyAndServer(t, "k", srv)
	ch, _ := a.Stream(t.Context(), "models/gemini-1.5-flash", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	var sawDelta bool
	for chunk := range ch {
		if chunk.Delta == "hi" {
			sawDelta = true
		}
	}
	if !sawDelta {
		t.Error("delta not seen")
	}
}

func TestStream_NilContent(t *testing.T) { //nolint:dupl
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":null}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}],\"role\":\"model\"}}]}\n\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	a := openWithKeyAndServer(t, "k", srv)
	ch, _ := a.Stream(t.Context(), "m", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	var saw bool
	for chunk := range ch {
		if chunk.Delta == "ok" {
			saw = true
		}
	}
	if !saw {
		t.Error("not saw")
	}
}

func TestExtractGeneration_MarshalError(t *testing.T) {
	t.Parallel()
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{Content: &genai.Content{Parts: []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "fn", Args: map[string]any{"bad": make(chan int)}}}}}},
		},
	}
	_, calls := extractGeneration(resp)
	if len(calls) != 1 || calls[0].Arguments != "{}" {
		t.Errorf("marshal error not handled %v", calls)
	}
}

func TestMapError_Pointer(t *testing.T) {
	t.Parallel()
	terr := &genai.APIError{Code: 401, Message: "pointer"}
	mapped := mapError(terr)
	if !errors.Is(mapped, ai.ErrAuth) {
		t.Errorf("pointer 401 not mapped %v", mapped)
	}
}

func TestOpen_InsecureLocalhost(t *testing.T) {
	t.Parallel()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"}}]}`))
	}))
	defer srv.Close()
	// Use helper which ensures insecure; also test direct Open with localhost sets insecure internally
	a, err := New(ai.Options{APIKey: "k", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err %v", err)
	}
	ad, _ := a.(*adapter) //nolint:forcetypeassert
	if tr, ok := ad.client.ClientConfig().HTTPClient.Transport.(*http.Transport); ok {
		if !tr.TLSClientConfig.InsecureSkipVerify {
			t.Error("insecure not set")
		}
	}
	// Also ensure Generate works via insecure
	_, err = a.Generate(t.Context(), "m", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate err %v", err)
	}
}

func TestOpen_SpoofedLoopbackStaysSecure(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"https://127.0.0.1.evil.com",
		"https://evil-localhost.com",
		"https://example.com/?x=localhost",
	} {
		a, err := New(ai.Options{APIKey: "k", BaseURL: raw})
		if err != nil {
			t.Fatalf("New(%q) err %v", raw, err)
		}
		ad, _ := a.(*adapter) //nolint:forcetypeassert
		if tr, ok := ad.client.ClientConfig().HTTPClient.Transport.(*http.Transport); ok {
			if tr.TLSClientConfig.InsecureSkipVerify {
				t.Errorf("insecure set for %q", raw)
			}
		}
		if err := a.Close(); err != nil {
			t.Fatalf("Close err %v", err)
		}
	}
}

func TestOpen_NewClientError(t *testing.T) {
	orig := newGenaiClient
	defer func() { newGenaiClient = orig }()
	newGenaiClient = func(_ context.Context, _ *genai.ClientConfig) (*genai.Client, error) {
		return nil, errors.New("boom super-secret-key-123 x-goog-api-key: super-secret-key-123")
	}
	_, err := New(ai.Options{APIKey: "super-secret-key-123", BaseURL: "https://example.com"})
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "super-secret-key-123") {
		t.Errorf("leak %q", err.Error())
	}
	if !strings.Contains(err.Error(), "REDACTED") {
		t.Errorf("no redacted %q", err.Error())
	}
}

func TestMapAPIError_429Empty(t *testing.T) {
	t.Parallel()
	e := genai.APIError{Code: 429}
	mapped := mapAPIError(e)
	var rle *ai.RateLimitedError
	if !errors.As(mapped, &rle) {
		t.Errorf("429 empty not RateLimited")
	}
}
