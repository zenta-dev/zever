//nolint:dupl
package openai

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/param"

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
		t.Fatalf("err %T not InvalidOptionsError", err)
	}

	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("err %q missing api_key", err.Error())
	}

	if strings.Contains(err.Error(), "xxxxx") {
		t.Errorf("err should not contain redacted marker unnecessarily")
	}
}

func TestOpen_BaseURLHTTPS(t *testing.T) {
	t.Parallel()

	_, err := New(ai.Options{APIKey: "sk-test", BaseURL: "http://example.com"})
	if !errors.Is(err, ai.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestOpen_Success(t *testing.T) {
	t.Parallel()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4o", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
}

func TestNewHTTPClient_CloneTLS12(t *testing.T) {
	// Checks DefaultTransport cloning: must not be parallel due to global read.
	c := newHTTPClient()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport %T not *http.Transport", c.Transport)
	}

	if tr.TLSClientConfig == nil {
		t.Fatalf("TLSClientConfig nil")
	}

	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want %v", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
}

func TestNewHTTPClient_Fallback(t *testing.T) {
	// Mutates global DefaultTransport: must not be parallel.
	prev := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(_ *http.Request) (*http.Response, error) { return nil, errors.New("fallback") })
	t.Cleanup(func() { http.DefaultTransport = prev })

	c := newHTTPClient()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport %T not *http.Transport", c.Transport)
	}

	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want %v", tr.TLSClientConfig.MinVersion, tls.VersionTLS12)
	}
}

func TestNewHTTPClient_UpgradeLegacyTLS(t *testing.T) {
	// Mutates global DefaultTransport: must not be parallel.
	prev := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS10}} //nolint:gosec
	t.Cleanup(func() { http.DefaultTransport = prev })

	c := newHTTPClient()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport %T not *http.Transport", c.Transport)
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %v, want TLS12", tr.TLSClientConfig.MinVersion)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestGenerate_ModelRequired(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("err = %v, want model required", err)
	}
}

func TestGenerate_ToolsAndResponseFormatConflict(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("should not reach server")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		Tools:          []ai.Tool{{Name: "tool1"}},
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	})
	if !errors.Is(err, ai.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
}

func TestGenerate_RoundtripSimple(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"hi there"},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":3}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	temp := float32(0.7)
	topP := float32(0.9)
	gen, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{MaxTokens: 50, Temperature: &temp, TopP: &topP})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if gen.Content != "hi there" {
		t.Fatalf("content = %q, want hi there", gen.Content)
	}

	if gen.Usage.PromptTokens != 8 || gen.Usage.CompletionTokens != 3 {
		t.Fatalf("usage = %+v", gen.Usage)
	}

	if gen.FinishReason != "stop" {
		t.Fatalf("finish = %q", gen.FinishReason)
	}

	if body["model"] != "gpt-4" {
		t.Errorf("model = %v, want gpt-4", body["model"])
	}

	if _, ok := body["temperature"]; !ok {
		t.Error("missing temperature")
	}

	if _, ok := body["max_tokens"]; !ok {
		t.Error("missing max_tokens")
	}

	if _, ok := body["top_p"]; !ok {
		t.Error("missing top_p")
	}
}

func TestGenerate_ModelOverride(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "gpt-4o-mini", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if body["model"] != "gpt-4o-mini" {
		t.Errorf("model = %v, want gpt-4o-mini", body["model"])
	}
}

func TestGenerate_WithTools(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"","tool_calls":[{"id":"call1","function":{"name":"get_weather","arguments":"{\"city\":\"SF\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	gen, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "weather"}}, ai.GenerateOptions{
		Tools:      []ai.Tool{{Name: "get_weather", Description: "get weather", Parameters: map[string]any{"type": "object", "properties": map[string]any{"city": map[string]any{"type": "string"}}}}},
		ToolChoice: ai.ToolChoiceAuto,
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if len(gen.ToolCalls) != 1 || gen.ToolCalls[0].Name != "get_weather" {
		t.Fatalf("toolCalls = %+v", gen.ToolCalls)
	}

	if _, ok := body["tools"]; !ok {
		t.Error("missing tools")
	}

	if _, ok := body["tool_choice"]; !ok {
		t.Error("missing tool_choice")
	}
}

func TestGenerate_ToolChoiceRequiredAndNone(t *testing.T) {
	t.Parallel()

	for _, tc := range []ai.ToolChoice{ai.ToolChoiceRequired, ai.ToolChoiceNone} {
		t.Run(string(tc), func(t *testing.T) {
			t.Parallel()

			var body map[string]any

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				_ = json.Unmarshal(b, &body)
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
			}))
			defer srv.Close()

			a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("Open err = %v", err)
			}
			defer func() { _ = a.Close() }()

			_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
				Tools:      []ai.Tool{{Name: "t"}},
				ToolChoice: tc,
			})
			if err != nil {
				t.Fatalf("Generate err = %v", err)
			}

			if _, ok := body["tool_choice"]; !ok {
				t.Error("missing tool_choice")
			}
		})
	}
}

func TestGenerate_ToolChoiceNamed(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		Tools:      []ai.Tool{{Name: "mytool"}},
		ToolChoice: ai.ToolChoice("tool:mytool"),
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	tc, ok := body["tool_choice"].(map[string]any)
	if !ok {
		t.Fatalf("tool_choice = %v, want object", body["tool_choice"])
	}

	// openai named tool choice is {"type":"function","function":{"name":"mytool"}}
	if tc["function"] == nil {
		t.Errorf("tool_choice missing function: %v", tc)
	}
}

func TestGenerate_ToolChoiceFallback(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		Tools:      []ai.Tool{{Name: "t"}},
		ToolChoice: ai.ToolChoice("custom"),
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if body["tool_choice"] == nil {
		t.Error("tool_choice missing for custom")
	}
}

func TestGenerate_ResponseFormatJSONObject(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"a\":1}"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	rf, ok := body["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("response_format = %v", body["response_format"])
	}

	if rf["type"] != "json_object" {
		t.Errorf("type = %v, want json_object", rf["type"])
	}
}

func TestGenerate_ResponseFormatJSONSchema(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{
			Type: "json_schema",
			JSONSchema: map[string]any{
				"name":   "my_schema",
				"schema": map[string]any{"type": "object"},
				"strict": true,
			},
		},
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	rf, ok := body["response_format"].(map[string]any)
	if !ok {
		t.Fatalf("response_format = %v", body["response_format"])
	}

	if rf["type"] != "json_schema" {
		t.Errorf("type = %v", rf["type"])
	}
}

func TestGenerate_ResponseFormatMapFallback(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	// Provide raw schema without wrapper keys
	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{
			Type:       "json_schema",
			JSONSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		},
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if body["response_format"] == nil {
		t.Error("missing response_format")
	}
}

func TestGenerate_ParallelToolCalls(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	disable := false
	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		Tools:             []ai.Tool{{Name: "t"}},
		ParallelToolCalls: &disable,
	})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	if body["parallel_tool_calls"] != false {
		t.Errorf("parallel_tool_calls = %v, want false", body["parallel_tool_calls"])
	}

	enable := true
	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		Tools:             []ai.Tool{{Name: "t"}},
		ParallelToolCalls: &enable,
	})
	if err != nil {
		t.Fatalf("Generate second err = %v", err)
	}
}

func TestGenerate_MessageRoles(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{
		{Role: ai.RoleSystem, Content: "sys"},
		{Role: ai.RoleUser, Content: "user"},
		{Role: ai.RoleAssistant, Content: "assistant"},
		{Role: ai.RoleTool, Content: "tool result", ToolCallID: "call1"},
		{Role: ai.RoleAssistant, Content: "with tools", ToolCalls: []ai.ToolCall{{ID: "c1", Name: "fn", Arguments: "{}"}}},
		{Role: "unknown", Content: "fallback"},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate err = %v", err)
	}

	msgs, ok := body["messages"].([]any)
	if !ok || len(msgs) != 6 {
		t.Fatalf("messages = %v", body["messages"])
	}
}

func TestGenerate_NoChoices(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("err = %v, want no choices", err)
	}
}

func TestGenerate_ErrorMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		status    int
		headers   map[string]string
		wantErr   error
		wantCheck func(error) bool
	}{
		{"401", 401, nil, ai.ErrAuth, func(err error) bool { return errors.Is(err, ai.ErrAuth) }},
		{"403", 403, nil, ai.ErrAuth, func(err error) bool { return errors.Is(err, ai.ErrAuth) }},
		{"429", 429, map[string]string{"Retry-After": "5"}, ai.ErrRateLimited, func(err error) bool {
			if !errors.Is(err, ai.ErrRateLimited) {
				return false
			}
			var re *ai.RateLimitedError
			if !errors.As(err, &re) {
				return false
			}
			return re.RetryAfter == 5*time.Second
		}},
		{"429_no_header", 429, nil, ai.ErrRateLimited, func(err error) bool { return errors.Is(err, ai.ErrRateLimited) }},
		{"400", 400, nil, ai.ErrInvalidRequest, func(err error) bool { return errors.Is(err, ai.ErrInvalidRequest) }},
		{"500", 500, nil, nil, func(err error) bool {
			return !errors.Is(err, ai.ErrAuth) && !errors.Is(err, ai.ErrRateLimited) && !errors.Is(err, ai.ErrInvalidRequest)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tc.headers {
					w.Header().Set(k, v)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprintf(w, `{"error":{"message":"boom","type":"%s","code":"err","param":""}}`, tc.name)
			}))
			defer srv.Close()

			a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
			if err != nil {
				t.Fatalf("Open err = %v", err)
			}
			defer func() { _ = a.Close() }()

			_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
			if err == nil {
				t.Fatalf("want error")
			}

			if !tc.wantCheck(err) {
				t.Fatalf("error check failed for %s: %v", tc.name, err)
			}

			if strings.Contains(err.Error(), "sk-test") {
				t.Errorf("error leaks api key: %v", err)
			}
		})
	}
}

func TestGenerate_Redaction(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key","type":"invalid_request_error","code":"invalid_api_key","param":""}}`))
	}))
	defer srv.Close()

	secret := "sk-super-secret-123"
	a, err := New(ai.Options{APIKey: secret, Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil {
		t.Fatal("want error")
	}

	if strings.Contains(err.Error(), secret) {
		t.Errorf("error leaks secret: %v", err)
	}
}

func TestEmbed_Dimensions(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2,0.3],"index":0}],"model":"text-embedding-3-small","usage":{"prompt_tokens":1,"total_tokens":1}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "text-embedding-3-small", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	vecs, err := a.Embed(context.Background(), "", []string{"hello"}, ai.EmbedOptions{Dimensions: 1536})
	if err != nil {
		t.Fatalf("Embed err = %v", err)
	}

	if len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Fatalf("vecs = %v", vecs)
	}

	if body["dimensions"] != float64(1536) {
		t.Errorf("dimensions = %v, want 1536", body["dimensions"])
	}
}

func TestEmbed_NoDimensions(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1],"index":0}]}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "emb", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Embed(context.Background(), "", []string{"a"}, ai.EmbedOptions{})
	if err != nil {
		t.Fatalf("Embed err = %v", err)
	}

	if _, ok := body["dimensions"]; ok {
		t.Errorf("dimensions should be absent, got %v", body["dimensions"])
	}
}

func TestEmbed_ModelRequired(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Embed(context.Background(), "", []string{"a"}, ai.EmbedOptions{})
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("err = %v, want model required", err)
	}
}

func TestEmbed_ErrorMapping(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad","type":"invalid","code":"err"}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "emb", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Embed(context.Background(), "", []string{"a"}, ai.EmbedOptions{})
	if !errors.Is(err, ai.ErrAuth) {
		t.Fatalf("err = %v, want ErrAuth", err)
	}
}

func TestStream_Ordering(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	var deltas []string

	var done bool

	var finish string

	for chunk := range ch {
		if chunk.Err != nil {
			t.Fatalf("chunk err = %v", chunk.Err)
		}

		if chunk.Delta != "" {
			deltas = append(deltas, chunk.Delta)
		}

		if chunk.Done {
			done = true
			finish = chunk.FinishReason
		}
	}

	if strings.Join(deltas, "") != "hello world" {
		t.Errorf("deltas = %q", strings.Join(deltas, ""))
	}

	if !done {
		t.Error("done not true")
	}

	if finish != "stop" {
		t.Errorf("finish = %q, want stop", finish)
	}
}

func TestStream_ToolCalls(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"call1\",\"function\":{\"name\":\"fn\",\"arguments\":\"{\\\"a\\\":1}\"}}]}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"tool_calls\"}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	found := false
	for chunk := range ch {
		if chunk.ToolCallID == "call1" && chunk.ToolName == "fn" {
			found = true
		}
	}

	if !found {
		t.Error("tool call not found in stream")
	}
}

// closeTrackingBody wraps an io.Reader as an io.ReadCloser that records
// whether Close was called.
type closeTrackingBody struct {
	io.Reader
	closed *atomic.Bool
}

func (c *closeTrackingBody) Close() error {
	c.closed.Store(true)
	return nil
}

// newStreamTestAdapter builds an adapter whose HTTP transport is a fake
// RoundTripper returning body as an SSE response with close tracking,
// bypassing httptest.NewServer so the exact same io.ReadCloser instance
// the SDK reads from is the one whose Close() we assert on (a real network
// round trip would let net/http wrap/replace the body).
func newStreamTestAdapter(t *testing.T, body string) (*adapter, *atomic.Bool) {
	t.Helper()

	closed := &atomic.Bool{}

	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       &closeTrackingBody{Reader: strings.NewReader(body), closed: closed},
			Request:    r,
		}, nil
	})

	client := openai.NewClient(option.WithAPIKey("sk-test"), option.WithHTTPClient(&http.Client{Transport: transport}))

	return &adapter{client: &client, model: "gpt-4"}, closed
}

// TestStream_ClosesBodyOnNormalCompletion is the regression test for the
// fixed stream leak: the original code never called stream.Close() on any
// exit path, including ordinary completion (stream.Next() returning false
// after [DONE]), not just cancellation.
func TestStream_ClosesBodyOnNormalCompletion(t *testing.T) {
	t.Parallel()

	a, closed := newStreamTestAdapter(t, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	for chunk := range ch {
		_ = chunk // drain until closed
	}

	if !closed.Load() {
		t.Error("response body not closed after normal stream completion")
	}
}

// TestStream_ClosesBodyOnCancel covers the early-return-on-ctx.Done() exit
// paths specifically.
func TestStream_ClosesBodyOnCancel(t *testing.T) {
	t.Parallel()

	// Two SSE events: Stream reads the first, we cancel before draining
	// the second, forcing the ctx.Done() branch inside the send select.
	a, closed := newStreamTestAdapter(t,
		"data: {\"choices\":[{\"delta\":{\"content\":\"one\"}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"content\":\"two\"}}]}\n\n"+
			"data: [DONE]\n\n")

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	<-ch // first chunk
	cancel()

	// Drain until the goroutine closes ch (its defers, including
	// stream.Close(), have then run).
	for chunk := range ch {
		_ = chunk // drain until closed
	}

	if !closed.Load() {
		t.Error("response body not closed after ctx cancellation mid-stream")
	}
}

func TestStream_ErrorMapping(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad","type":"invalid","code":"err"}}`))
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	// Stream should emit error chunk eventually (via stream.Err)
	for chunk := range ch {
		if chunk.Err != nil {
			if !errors.Is(chunk.Err, ai.ErrAuth) {
				t.Fatalf("chunk err = %v, want ErrAuth", chunk.Err)
			}

			return
		}
	}

	t.Error("want error chunk")
}

func TestStream_ModelRequired(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "model is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestStream_ToolsConflict(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	_, err = a.Stream(context.Background(), "", nil, ai.GenerateOptions{
		Tools:          []ai.Tool{{Name: "t"}},
		ResponseFormat: &ai.ResponseFormat{Type: "json_object"},
	})
	if !errors.Is(err, ai.ErrNotSupported) {
		t.Fatalf("err = %v", err)
	}
}

func TestStream_WithResponseFormatAndParallel(t *testing.T) {
	t.Parallel()

	var body map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()

	disable := false
	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		ResponseFormat:    &ai.ResponseFormat{Type: "json_object"},
		ParallelToolCalls: &disable,
	})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}

	for range ch {
		_ = struct{}{}
	}

	if body["response_format"] == nil {
		t.Error("missing response_format in stream request")
	}

	if body["parallel_tool_calls"] != false {
		t.Errorf("parallel_tool_calls = %v, want false", body["parallel_tool_calls"])
	}
}

func TestSafeIntOverflow(t *testing.T) {
	t.Parallel()

	_, err := safeInt64ToInt(math.MaxInt64)
	if err == nil {
		t.Fatal("want overflow error")
	}

	_, err = safeInt64ToInt(math.MinInt64)
	if err == nil && math.MinInt64 < int64(math.MinInt) {
		t.Fatal("want overflow")
	}

	v, err := safeInt64ToInt(42)
	if err != nil || v != 42 {
		t.Fatalf("safeInt 42 = %d err %v", v, err)
	}
}

func TestMapErrorNil(t *testing.T) {
	t.Parallel()

	if mapError(nil) != nil {
		t.Error("mapError(nil) != nil")
	}
}

func TestParseRetryAfter(t *testing.T) {
	t.Parallel()

	if d := parseRetryAfter(nil); d != 0 {
		t.Errorf("nil = %v want 0", d)
	}

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://example.com", nil)

	resp := &http.Response{Header: http.Header{}}
	resp.Header.Set("Retry-After", "10")
	if d := parseRetryAfter(resp); d != 10*time.Second {
		t.Errorf("secs = %v want 10s", d)
	}

	resp.Header.Set("Retry-After", "not-a-number")
	_ = req
	if d := parseRetryAfter(resp); d != 0 {
		t.Errorf("invalid = %v want 0", d)
	}

	// Date format: use future date
	future := time.Now().Add(2 * time.Second).UTC().Format(http.TimeFormat)
	resp.Header.Set("Retry-After", future)
	if d := parseRetryAfter(resp); d == 0 {
		t.Error("future date should give duration")
	}

	// Past date gives 0
	past := time.Now().Add(-2 * time.Hour).UTC().Format(http.TimeFormat)
	resp.Header.Set("Retry-After", past)
	if d := parseRetryAfter(resp); d != 0 {
		t.Errorf("past = %v want 0", d)
	}
}

func TestClose(t *testing.T) {
	t.Parallel()

	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}

	if err := a.Close(); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
}

func TestBuildToolChoiceHelper(t *testing.T) {
	t.Parallel()

	for _, in := range []string{"auto", "required", "none", "tool:mytool", "custom"} {
		p := buildToolChoice(in)
		if param.IsOmitted(p.OfAuto) && p.OfFunctionToolChoice == nil {
			t.Errorf("buildToolChoice(%q) nil", in)
		}
	}
}

func TestBuildMessagesHelper(t *testing.T) {
	t.Parallel()

	msgs := buildMessages([]ai.Message{
		{Role: ai.RoleSystem, Content: "sys"},
		{Role: ai.RoleUser, Content: "user"},
		{Role: ai.RoleAssistant, Content: "asst"},
		{Role: ai.RoleTool, Content: "tool", ToolCallID: "id"},
	})
	if len(msgs) != 4 {
		t.Fatalf("len = %d", len(msgs))
	}
}

func TestBuildToolsHelper(t *testing.T) {
	t.Parallel()

	tools := buildTools([]ai.Tool{{Name: "t", Description: "d", Parameters: map[string]any{"type": "object"}}})
	if len(tools) != 1 || tools[0].OfFunction == nil || tools[0].OfFunction.Function.Name != "t" {
		t.Fatalf("tools = %+v", tools)
	}
}

func TestApplyResponseFormatHelper(t *testing.T) {
	t.Parallel()

	var p openai.ChatCompletionNewParams

	applyResponseFormat(&p, &ai.ResponseFormat{Type: "json_object"})
	if p.ResponseFormat.OfJSONObject == nil {
		t.Error("OfJSONObject nil")
	}

	var p2 openai.ChatCompletionNewParams
	applyResponseFormat(&p2, &ai.ResponseFormat{Type: "json_schema", JSONSchema: map[string]any{"name": "x", "schema": map[string]any{"type": "object"}, "strict": true}})
	if p2.ResponseFormat.OfJSONSchema == nil {
		t.Error("OfJSONSchema nil")
	}

	var p3 openai.ChatCompletionNewParams
	applyResponseFormat(&p3, &ai.ResponseFormat{JSONSchema: map[string]any{"type": "object"}})
	if p3.ResponseFormat.OfJSONSchema == nil {
		t.Error("p3 nil")
	}
}

func TestValidateOptions_Branches(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		opts ai.Options
	}{
		{"timeout", ai.Options{APIKey: "k", Timeout: -1}},
		{"invalid url", ai.Options{APIKey: "k", BaseURL: "http://[::1"}},
		{"no scheme", ai.Options{APIKey: "k", BaseURL: "example.com/api"}},
		{"no host", ai.Options{APIKey: "k", BaseURL: "https:///path"}},
		{"http scheme", ai.Options{APIKey: "k", BaseURL: "http://example.com"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := New(tc.opts)
			if err == nil {
				t.Fatalf("want error for %s", tc.name)
			}
			if !errors.Is(err, ai.ErrInvalidOptions) {
				t.Fatalf("err = %v want ErrInvalidOptions", err)
			}
		})
	}

	// valid loopback http should succeed
	if _, err := New(ai.Options{APIKey: "k", BaseURL: "http://127.0.0.1:8080"}); err != nil {
		t.Fatalf("loopback http should be allowed, got %v", err)
	}
}

func TestNewHTTPClient_NilTLSConfig(t *testing.T) {
	// Mutates global DefaultTransport: must not be parallel.
	prev := http.DefaultTransport
	http.DefaultTransport = &http.Transport{TLSClientConfig: nil}
	t.Cleanup(func() { http.DefaultTransport = prev })
	c := newHTTPClient()
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport %T", c.Transport)
	}
	if tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS not set correctly: %+v", tr.TLSClientConfig)
	}
}

func TestNewHTTPClientFromTransport(t *testing.T) {
	t.Parallel()
	// nil transport
	c := newHTTPClientFromTransport(nil)
	if tr, ok := c.Transport.(*http.Transport); !ok || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("nil transport not correct")
	}
	// nil TLSConfig
	c = newHTTPClientFromTransport(&http.Transport{TLSClientConfig: nil})
	if tr, ok := c.Transport.(*http.Transport); !ok || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("nil TLSConfig not correct")
	}
	// legacy TLS
	c = newHTTPClientFromTransport(&http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS10}}) //nolint:gosec
	if tr, ok := c.Transport.(*http.Transport); !ok || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("legacy TLS not upgraded")
	}
	// already TLS12
	c = newHTTPClientFromTransport(&http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}})
	if tr, ok := c.Transport.(*http.Transport); !ok || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS12 not preserved")
	}
	// TLS13 should stay TLS13 (since we set unconditional to TLS12, it will downgrade, but we now set unconditional to TLS12, so it will be TLS12)
	// This test ensures unconditional path is covered
}

func TestGenerate_OverflowPromptTokens(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5000000000,"completion_tokens":1}}`))
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "prompt_tokens") {
		t.Fatalf("want prompt_tokens overflow, got %v", err)
	}
}

func TestGenerate_OverflowCompletionTokens(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":5000000000}}`))
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "completion_tokens") {
		t.Fatalf("want completion overflow, got %v", err)
	}
}

func TestGenerate_NilResponse(t *testing.T) {
	// Mutates global newHTTPClient: must not be parallel.
	orig := newHTTPClient
	newHTTPClient = func() *http.Client {
		return &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
			return nil, nil
		})}
	}
	t.Cleanup(func() { newHTTPClient = orig })
	// We need to create adapter with that client; Open will use patched newHTTPClient
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	// Use non-loopback https URL to pass validation but client will not hit server due to nil transport
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	_, err = a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil {
		t.Fatalf("want error for nil response")
	}
	// Could be "no response" or wrapped error, but should contain something
	if !strings.Contains(err.Error(), "no response") && !strings.Contains(err.Error(), "generate") {
		t.Fatalf("err = %v", err)
	}
}

func TestStream_AllOptions(t *testing.T) {
	t.Parallel()
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	temp := float32(0.5)
	topP := float32(0.9)
	disable := false
	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		Temperature:       &temp,
		MaxTokens:         10,
		TopP:              &topP,
		Tools:             []ai.Tool{{Name: "t"}},
		ToolChoice:        ai.ToolChoiceAuto,
		ParallelToolCalls: &disable,
	})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	for range ch {
		_ = struct{}{}
	}
	if body["temperature"] == nil || body["max_tokens"] == nil || body["top_p"] == nil || body["tools"] == nil || body["tool_choice"] == nil || body["parallel_tool_calls"] == nil {
		t.Fatalf("missing fields in stream request: %v", body)
	}
}

func TestStream_EmptyChoicesContinue(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	found := false
	for c := range ch {
		if c.Delta == "hi" {
			found = true
		}
	}
	if !found {
		t.Error("did not receive hi after empty choices")
	}
}

func TestStream_EmptyToolCallSkipped(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// tool call with empty fields should be skipped
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"\",\"function\":{\"name\":\"\",\"arguments\":\"\"}}]}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	hasTool := false
	for c := range ch {
		if c.ToolCallID != "" || c.ToolName != "" {
			hasTool = true
		}
	}
	if hasTool {
		t.Error("empty tool call should be skipped")
	}
}

func TestStream_ContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// slow stream
		for i := 0; i < 5; i++ {
			_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			time.Sleep(50 * time.Millisecond)
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	// cancel immediately, should cause goroutine to exit via ctx.Done branches
	cancel()
	// Drain or timeout
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
	}
	// Ensure channel eventually closes
	done := make(chan struct{})
	go func() {
		for range ch {
			_ = struct{}{}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("stream channel did not close after cancel")
	}
}

func TestApplyResponseFormat_StrictFalse(t *testing.T) {
	t.Parallel()
	var p openai.ChatCompletionNewParams
	applyResponseFormat(&p, &ai.ResponseFormat{JSONSchema: map[string]any{"name": "", "schema": map[string]any{"type": "object"}}})
	if p.ResponseFormat.OfJSONSchema == nil {
		t.Error("want json schema")
	}
	// strict false case already covered, but ensure no panic for empty name
	if p.ResponseFormat.OfJSONSchema.JSONSchema.Name != "response" {
		t.Errorf("name = %q want response", p.ResponseFormat.OfJSONSchema.JSONSchema.Name)
	}
}

func TestApplyResponseFormat_EmptySchemaFallback(t *testing.T) {
	t.Parallel()
	var p openai.ChatCompletionNewParams
	// Only name and strict, no other keys -> len(schema)==0 after loop -> fallback to whole map
	applyResponseFormat(&p, &ai.ResponseFormat{JSONSchema: map[string]any{"name": "x", "strict": true}})
	if p.ResponseFormat.OfJSONSchema == nil {
		t.Fatal("want json schema")
	}
	// schema will be the whole map due to fallback
	if p.ResponseFormat.OfJSONSchema.JSONSchema.Name != "x" {
		t.Errorf("name = %q want x", p.ResponseFormat.OfJSONSchema.JSONSchema.Name)
	}
}

func TestClose_Idempotent(t *testing.T) {
	t.Parallel()
	a, err := New(ai.Options{APIKey: "k", Model: "m", BaseURL: "https://example.com"})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("Close err = %v", err)
	}
	if err := a.Close(); err != nil {
		t.Fatalf("second Close err = %v", err)
	}
}

func TestStream_CtxDoneDuringDelta(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 40; i++ {
			_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	// cancel before draining to trigger ctx.Done branch
	cancel()
	// Give goroutine time to hit select
	time.Sleep(100 * time.Millisecond)
	// Drain remaining to avoid goroutine leak
	go func() {
		for range ch {
			_ = struct{}{}
		}
	}()
	time.Sleep(100 * time.Millisecond)
}

func TestStream_CtxDoneDuringToolCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for i := 0; i < 40; i++ {
			_, _ = fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"id\":\"id%d\",\"function\":{\"name\":\"fn\",\"arguments\":\"{}\"}}]}}]}\n\n", i)
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer srv.Close()
	a, err := New(ai.Options{APIKey: "sk-test", Model: "gpt-4", BaseURL: srv.URL})
	if err != nil {
		t.Fatalf("Open err = %v", err)
	}
	defer func() { _ = a.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream err = %v", err)
	}
	cancel()
	time.Sleep(100 * time.Millisecond)
	go func() {
		for range ch {
			_ = struct{}{}
		}
	}()
	time.Sleep(100 * time.Millisecond)
}
