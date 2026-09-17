package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/ai"
)

// fakeTransport hand-fakes the HTTP layer: no network is touched.
type fakeTransport func(*http.Request) (*http.Response, error)

func (f fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResp(status int, v any) *http.Response {
	body, _ := json.Marshal(v)

	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func chatOK(content string, prompt, eval int) map[string]any {
	return map[string]any{
		"message":           map[string]any{"content": content},
		"prompt_eval_count": prompt,
		"eval_count":        eval,
	}
}

func openFake(t *testing.T, tr fakeTransport, model string) ai.AI {
	t.Helper()

	a, err := Open(Options{Addr: "http://localhost:11434", Model: model, Transport: tr})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	return a
}

func TestOpen_defaults(t *testing.T) {
	t.Parallel()

	a, err := Open(Options{})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ad, ok := a.(*adapter)
	if !ok {
		t.Fatalf("Open returned %T, want *adapter", a)
	}

	if ad.addr != DefaultAddr {
		t.Fatalf("addr = %q, want %q", ad.addr, DefaultAddr)
	}

	if ad.requestTimeout() != DefaultTimeout {
		t.Fatalf("timeout = %v, want %v", ad.requestTimeout(), DefaultTimeout)
	}
}

func TestOpen_custom(t *testing.T) {
	t.Parallel()

	a, err := Open(Options{Addr: "http://ollama:11434", Model: "llama3", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	ad, ok := a.(*adapter)
	if !ok {
		t.Fatalf("Open = %T, want *adapter", a)
	}
	if ad.addr != "http://ollama:11434" {
		t.Fatalf("addr = %q", ad.addr)
	}

	if ad.defaultModel != "llama3" {
		t.Fatalf("model = %q", ad.defaultModel)
	}

	if ad.requestTimeout() != 5*time.Second {
		t.Fatalf("timeout = %v", ad.requestTimeout())
	}
}

func TestOpen_invalid(t *testing.T) {
	t.Parallel()

	if _, err := Open(Options{Timeout: -time.Second}); err == nil {
		t.Fatal("expected error, got nil")
	}

	if _, err := Open(Options{Addr: "ftp://example.com"}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGenerate_happyPath(t *testing.T) {
	t.Parallel()

	var gotReq chatRequest

	a := openFake(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/chat" {
			return nil, fmt.Errorf("unexpected path %s", r.URL.Path)
		}

		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			return nil, err
		}

		return jsonResp(http.StatusOK, chatOK("hello", 5, 10)), nil
	}, "llama3")

	gen, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if gen.Content != "hello" {
		t.Fatalf("content = %q, want hello", gen.Content)
	}

	if gen.Usage.PromptTokens != 5 || gen.Usage.CompletionTokens != 10 {
		t.Fatalf("usage = %+v, want 5/10", gen.Usage)
	}

	if gotReq.Model != "llama3" {
		t.Fatalf("model = %q, want llama3", gotReq.Model)
	}

	if gotReq.Stream {
		t.Fatal("stream = true, want false")
	}
}

func TestGenerate_explicitModelWins(t *testing.T) {
	t.Parallel()

	var gotReq chatRequest

	a := openFake(t, func(r *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		return jsonResp(http.StatusOK, chatOK("ok", 0, 0)), nil
	}, "default-model")

	_, err := a.Generate(context.Background(), "explicit", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if gotReq.Model != "explicit" {
		t.Fatalf("model = %q, want explicit", gotReq.Model)
	}
}

func TestGenerate_noModel(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, chatOK("x", 0, 0)), nil
	}, "")

	_, err := a.Generate(context.Background(), "", nil, ai.GenerateOptions{})
	if !errors.Is(err, ErrNoModel) {
		t.Fatalf("err = %v, want ErrNoModel", err)
	}
}

func TestStream_noModel(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "")

	_, err := a.Stream(context.Background(), "", nil, ai.GenerateOptions{})
	if !errors.Is(err, ErrNoModel) {
		t.Fatalf("err = %v, want ErrNoModel", err)
	}
}

func TestEmbed_noModel(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "")

	_, err := a.Embed(context.Background(), "", []string{"hi"}, ai.EmbedOptions{})
	if !errors.Is(err, ErrNoModel) {
		t.Fatalf("err = %v, want ErrNoModel", err)
	}
}

func TestGenerate_normalizesRoles(t *testing.T) {
	t.Parallel()

	var gotReq chatRequest

	a := openFake(t, func(r *http.Request) (*http.Response, error) {
		_ = json.NewDecoder(r.Body).Decode(&gotReq)
		return jsonResp(http.StatusOK, chatOK("ok", 0, 0)), nil
	}, "llama3")

	_, err := a.Generate(context.Background(), "", []ai.Message{
		{Role: ai.RoleSystem, Content: "sys"},
		{Role: ai.RoleAssistant, Content: "a"},
		{Role: ai.RoleUser, Content: "u"},
		{Role: "machine", Content: "m"},
		{Role: ai.RoleTool, Content: "t"},
	}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	want := []string{"system", "assistant", "user", "user", "user"}
	if len(gotReq.Messages) != len(want) {
		t.Fatalf("messages = %+v, want %d", gotReq.Messages, len(want))
	}

	for i, role := range want {
		if gotReq.Messages[i].Role != role {
			t.Fatalf("message %d role = %q, want %q", i, gotReq.Messages[i].Role, role)
		}
	}
}

func TestGenerate_optionsMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		opts  ai.GenerateOptions
		check func(t *testing.T, req chatRequest)
	}{
		{
			name: "max tokens",
			opts: ai.GenerateOptions{MaxTokens: 100},
			check: func(t *testing.T, req chatRequest) {
				t.Helper()

				if req.Options == nil || req.Options.NumPredict != 100 {
					t.Fatalf("options = %+v, want num_predict 100", req.Options)
				}
			},
		},
		{
			name: "temperature and top_p",
			opts: ai.GenerateOptions{Temperature: ptrFloat32(0.5), TopP: ptrFloat32(0.9)},
			check: func(t *testing.T, req chatRequest) {
				t.Helper()

				if req.Options == nil || *req.Options.Temperature != 0.5 || *req.Options.TopP != 0.9 {
					t.Fatalf("options = %+v", req.Options)
				}
			},
		},
		{
			name: "no options",
			opts: ai.GenerateOptions{},
			check: func(t *testing.T, req chatRequest) {
				t.Helper()

				if req.Options != nil {
					t.Fatalf("options = %+v, want nil", req.Options)
				}
			},
		},
		{
			name: "json format",
			opts: ai.GenerateOptions{ResponseFormat: &ai.ResponseFormat{Type: "json"}},
			check: func(t *testing.T, req chatRequest) {
				t.Helper()

				if req.Format != "json" {
					t.Fatalf("format = %v, want json", req.Format)
				}
			},
		},
		{
			name: "schema format",
			opts: ai.GenerateOptions{ResponseFormat: &ai.ResponseFormat{
				Type:       "json_schema",
				JSONSchema: map[string]any{"schema": map[string]any{"type": "object"}},
			}},
			check: func(t *testing.T, req chatRequest) {
				t.Helper()

				m, ok := req.Format.(map[string]any)
				if !ok || m["type"] != "object" {
					t.Fatalf("format = %v", req.Format)
				}
			},
		},
		{
			name: "tools",
			opts: ai.GenerateOptions{Tools: []ai.Tool{{Name: "get_time", Description: "clock"}}},
			check: func(t *testing.T, req chatRequest) {
				t.Helper()

				if len(req.Tools) != 1 || req.Tools[0].Type != "function" || req.Tools[0].Function.Name != "get_time" {
					t.Fatalf("tools = %+v", req.Tools)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotReq chatRequest

			a := openFake(t, func(r *http.Request) (*http.Response, error) {
				_ = json.NewDecoder(r.Body).Decode(&gotReq)
				return jsonResp(http.StatusOK, chatOK("ok", 0, 0)), nil
			}, "llama3")

			if _, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, tt.opts); err != nil {
				t.Fatalf("Generate: %v", err)
			}

			tt.check(t, gotReq)
		})
	}
}

func ptrFloat32(f float32) *float32 { return &f }

func TestGenerate_toolCalls(t *testing.T) {
	t.Parallel()

	resp := map[string]any{
		"message": map[string]any{
			"content": "",
			"tool_calls": []any{
				map[string]any{"function": map[string]any{"name": "get_time", "arguments": map[string]any{"tz": "UTC"}}},
				map[string]any{"function": map[string]any{"name": "noop"}},
			},
		},
	}

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusOK, resp), nil
	}, "llama3")

	gen, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if len(gen.ToolCalls) != 2 {
		t.Fatalf("tool calls = %+v, want 2", gen.ToolCalls)
	}

	if gen.ToolCalls[0].ID != "call_0" || gen.ToolCalls[0].Name != "get_time" {
		t.Fatalf("call 0 = %+v", gen.ToolCalls[0])
	}

	if !strings.Contains(gen.ToolCalls[0].Arguments, "UTC") {
		t.Fatalf("arguments = %q, want tz", gen.ToolCalls[0].Arguments)
	}

	if gen.ToolCalls[1].Arguments != "{}" {
		t.Fatalf("arguments = %q, want {}", gen.ToolCalls[1].Arguments)
	}
}

func TestGenerate_errorPaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		handler func() *http.Response
		want    string
	}{
		{
			name:    "status error",
			handler: func() *http.Response { return jsonResp(http.StatusBadRequest, map[string]any{"error": "bad"}) },
			want:    "status 400",
		},
		{
			name: "bad json",
			handler: func() *http.Response {
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{oops"))}
			},
			want: "unmarshal response",
		},
		{
			name: "oversized",
			handler: func() *http.Response {
				big := `{"message":{"content":"` + strings.Repeat("a", maxResponseBytes) + `"}}`
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(big))}
			},
			want: "exceeds",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			a := openFake(t, func(_ *http.Request) (*http.Response, error) {
				return tt.handler(), nil
			}, "llama3")

			_, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestGenerate_transportError(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("dial boom")
	}, "llama3")

	_, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("err = %v, want request failed", err)
	}
}

func TestEmbed_happyPath(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/api/embed" {
			return nil, fmt.Errorf("unexpected path %s", r.URL.Path)
		}

		return jsonResp(http.StatusOK, map[string]any{"embeddings": [][]float32{{0.1, 0.2, 0.3}}}), nil
	}, "llama3")

	vecs, err := a.Embed(context.Background(), "", []string{"hello"}, ai.EmbedOptions{})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	if len(vecs) != 1 || len(vecs[0]) != 3 {
		t.Fatalf("embeddings = %v, want 1x3", vecs)
	}
}

func TestEmbed_dimensions_notSupported(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "llama3")

	_, err := a.Embed(context.Background(), "", []string{"hello"}, ai.EmbedOptions{Dimensions: 256})
	if !errors.Is(err, ai.ErrNotSupported) {
		t.Fatalf("err = %v, want ErrNotSupported", err)
	}
}

func TestEmbed_statusError(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusInternalServerError, map[string]any{"error": "boom"}), nil
	}, "llama3")

	_, err := a.Embed(context.Background(), "", []string{"hi"}, ai.EmbedOptions{})
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("err = %v, want status 500", err)
	}
}

func TestStream_happyPath(t *testing.T) {
	t.Parallel()

	ndjson := "{\"message\":{\"content\":\"hi\"},\"done\":false}\n" +
		"{\"message\":{\"content\":\" there\"},\"done\":false}\n" +
		"{\"message\":{\"content\":\"\"},\"done\":true,\"eval_count\":3,\"prompt_eval_count\":5}\n"

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(ndjson))}, nil
	}, "llama3")

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var deltas strings.Builder

	var done bool

	var usage *ai.Usage

	for c := range ch {
		if c.Err != nil {
			t.Fatalf("chunk err: %v", c.Err)
		}

		deltas.WriteString(c.Delta)

		if c.Done {
			done = true
			usage = c.Usage
		}
	}

	if deltas.String() != "hi there" {
		t.Fatalf("deltas = %q, want %q", deltas.String(), "hi there")
	}

	if !done {
		t.Fatal("missing Done chunk")
	}

	if usage == nil || usage.PromptTokens != 5 || usage.CompletionTokens != 3 {
		t.Fatalf("usage = %+v, want 5/3", usage)
	}
}

func TestStream_toolCalls(t *testing.T) {
	t.Parallel()

	ndjson := "{\"message\":{\"content\":\"\",\"tool_calls\":[{\"function\":{\"name\":\"warming\",\"arguments\":{}}}]},\"done\":false}\n" +
		"{\"message\":{\"content\":\"\",\"tool_calls\":[{\"function\":{\"name\":\"get_time\",\"arguments\":{\"tz\":\"UTC\"}}}]},\"done\":true,\"eval_count\":1,\"prompt_eval_count\":2}\n"

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(ndjson))}, nil
	}, "llama3")

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var toolSeen, doneSeen bool

	for c := range ch {
		if c.Err != nil {
			t.Fatalf("chunk err: %v", c.Err)
		}

		if c.ToolName == "get_time" {
			toolSeen = true

			if c.ToolCallID != "call_0" || !strings.Contains(c.ToolArgsDelta, "UTC") {
				t.Fatalf("tool chunk = %+v", c)
			}
		}

		if c.Done {
			doneSeen = true
		}
	}

	if !toolSeen {
		t.Fatal("missing tool chunk")
	}

	if !doneSeen {
		t.Fatal("missing Done chunk")
	}
}

func TestStream_serverErrorPayload(t *testing.T) {
	t.Parallel()

	ndjson := "{\"error\":\"model missing\"}\n"

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(ndjson))}, nil
	}, "llama3")

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sawErr bool

	for c := range ch {
		if c.Err != nil {
			sawErr = true

			if !strings.Contains(c.Err.Error(), "stream error") {
				t.Fatalf("err = %v", c.Err)
			}
		}
	}

	if !sawErr {
		t.Fatal("expected stream error chunk")
	}
}

func TestStream_statusError(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return jsonResp(http.StatusUnauthorized, map[string]any{"error": "nope"}), nil
	}, "llama3")

	_, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("err = %v, want status 401", err)
	}
}

func TestStream_decodeError(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{oops\n"))}, nil
	}, "llama3")

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var sawErr bool

	for c := range ch {
		if c.Err != nil {
			sawErr = true
		}
	}

	if !sawErr {
		t.Fatal("expected decode error chunk")
	}
}

func TestClose_nilError(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "llama3")

	if err := a.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestPostRequest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr string
	}{
		{name: "simple", addr: "http://localhost:11434", want: "http://localhost:11434/api/chat"},
		{name: "trailing slash", addr: "http://localhost:11434/", want: "http://localhost:11434/api/chat"},
		{name: "unparseable", addr: "http://[::1", wantErr: "create request"},
		{name: "bad scheme", addr: "ftp://example.com", wantErr: "http or https"},
		{name: "no host", addr: "https:///chat", wantErr: "must have a host"},
		{name: "user info", addr: "http://user@example.com", wantErr: "user info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req, err := postRequest(context.Background(), tt.addr, "/api/chat", []byte("{}"))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("postRequest: %v", err)
			}

			if req.URL.String() != tt.want {
				t.Fatalf("url = %q, want %q", req.URL.String(), tt.want)
			}

			if req.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("content-type = %q", req.Header.Get("Content-Type"))
			}

			if req.Method != http.MethodPost {
				t.Fatalf("method = %q", req.Method)
			}
		})
	}
}

func TestCheckEndpoint(t *testing.T) {
	t.Parallel()

	mustParse := func(s string) *url.URL {
		u, err := url.Parse(s)
		if err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}

		return u
	}

	tests := []struct {
		name    string
		raw     string
		wantErr string
	}{
		{name: "http ok", raw: "http://example.com/api/chat"},
		{name: "https ok", raw: "https://example.com/api/chat"},
		{name: "bad scheme", raw: "ftp://example.com/x", wantErr: "http or https"},
		{name: "no host", raw: "https:///chat", wantErr: "must have a host"},
		{name: "user info", raw: "http://user@example.com/x", wantErr: "user info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := checkEndpoint(mustParse(tt.raw))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("checkEndpoint: %v", err)
			}
		})
	}
}

func TestEncode_error(t *testing.T) {
	t.Parallel()

	if _, err := encode(func() {}); err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("err = %v, want marshal request", err)
	}

	if _, err := encode(map[string]any{"ok": true}); err != nil {
		t.Fatalf("encode: %v", err)
	}
}

func TestGenerate_unmarshalableFormat(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "llama3")

	_, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{JSONSchema: map[string]any{"schema": map[string]any{"f": func() {}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("err = %v, want marshal request", err)
	}
}

func TestStream_unmarshalableFormat(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "llama3")

	_, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{
		ResponseFormat: &ai.ResponseFormat{JSONSchema: map[string]any{"schema": map[string]any{"f": func() {}}}},
	})
	if err == nil || !strings.Contains(err.Error(), "marshal request") {
		t.Fatalf("err = %v, want marshal request", err)
	}
}

func TestGenerate_badAddr(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "llama3")
	ad, ok := a.(*adapter)
	if !ok {
		t.Fatalf("openFake = %T, want *adapter", a)
	}
	ad.addr = "http://[::1"

	_, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("err = %v, want create request", err)
	}
}

func TestStream_badAddr(t *testing.T) {
	t.Parallel()

	a := openFake(t, nil, "llama3")
	ad, ok := a.(*adapter)
	if !ok {
		t.Fatalf("openFake = %T, want *adapter", a)
	}
	ad.addr = "http://[::1"

	_, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "create request") {
		t.Fatalf("err = %v, want create request", err)
	}
}

func TestStream_transportError(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("dial boom")
	}, "llama3")

	_, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("err = %v, want request failed", err)
	}
}

type errReader struct{}

func (errReader) Read(_ []byte) (int, error) { return 0, errors.New("read boom") }

func TestGenerate_readError(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(errReader{})}, nil
	}, "llama3")

	_, err := a.Generate(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err == nil || !strings.Contains(err.Error(), "read response") {
		t.Fatalf("err = %v, want read response", err)
	}
}

func TestEmbed_badJSON(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("{oops"))}, nil
	}, "llama3")

	_, err := a.Embed(context.Background(), "", []string{"hi"}, ai.EmbedOptions{})
	if err == nil || !strings.Contains(err.Error(), "unmarshal embed response") {
		t.Fatalf("err = %v, want unmarshal embed response", err)
	}
}

func TestEmbed_transportError(t *testing.T) {
	t.Parallel()

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return nil, errors.New("dial boom")
	}, "llama3")

	_, err := a.Embed(context.Background(), "", []string{"hi"}, ai.EmbedOptions{})
	if err == nil || !strings.Contains(err.Error(), "embed") {
		t.Fatalf("err = %v, want embed", err)
	}
}

func TestToolCalls_marshalError(t *testing.T) {
	t.Parallel()

	var tc ollamaToolCall

	tc.Function.Name = "bad"
	tc.Function.Arguments = map[string]any{"f": func() {}}

	if _, err := toolCalls([]ollamaToolCall{tc}); err == nil || !strings.Contains(err.Error(), "marshal tool arguments") {
		t.Fatalf("err = %v, want marshal tool arguments", err)
	}

	if _, err := toolCalls(nil); err != nil {
		t.Fatalf("toolCalls(nil): %v", err)
	}
}

func TestStream_eofCloses(t *testing.T) {
	t.Parallel()

	ndjson := "{\"message\":{\"content\":\"partial\"},\"done\":false}\n"

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(ndjson))}, nil
	}, "llama3")

	ch, err := a.Stream(context.Background(), "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	var deltas string

	var open = true

	timeout := time.After(5 * time.Second)
	for open {
		select {
		case c, ok := <-ch:
			if !ok {
				open = false
				break
			}

			if c.Err != nil {
				t.Fatalf("chunk err: %v", c.Err)
			}

			deltas += c.Delta
		case <-timeout:
			t.Fatal("stream did not close on EOF")
		}
	}

	if deltas != "partial" {
		t.Fatalf("deltas = %q, want partial", deltas)
	}
}

// openStreamBlocked starts a stream with a live context and no reader,
// waiting until the writer fills the 32-slot buffer and blocks on its next send.
func openStreamBlocked(t *testing.T, body string) (<-chan ai.StreamChunk, context.CancelFunc) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())

	a := openFake(t, func(_ *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body))}, nil
	}, "llama3")

	ch, err := a.Stream(ctx, "", []ai.Message{{Role: ai.RoleUser, Content: "hi"}}, ai.GenerateOptions{})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for len(ch) < 32 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("writer did not fill buffer")
		}

		time.Sleep(time.Millisecond)
	}

	return ch, cancel
}

// drainClosed drains ch until close, asserting no error chunks arrive.
func drainClosed(t *testing.T, ch <-chan ai.StreamChunk, allowErr bool) {
	t.Helper()

	// At least the 32 buffered chunks must arrive: after cancel the
	// writer may still win a send-vs-Done select and deliver trailing
	// chunks before noticing cancellation, so an exact count is racy by
	// construction. Closure (no leak, no stall) is what is asserted.
	const minChunks = 32

	var n int

	timeout := time.After(5 * time.Second)
	for {
		select {
		case c, ok := <-ch:
			if !ok {
				if n < minChunks {
					t.Fatalf("chunks = %d, want at least %d", n, minChunks)
				}

				return
			}

			if c.Err != nil && !allowErr {
				t.Fatalf("chunk err: %v", c.Err)
			}

			n++
		case <-timeout:
			t.Fatal("stream did not close after cancel")
		}
	}
}

func TestStream_cancelWhileBlocked_plain(t *testing.T) {
	line := "{\"message\":{\"content\":\"x\"},\"done\":false}\n"
	ch, cancel := openStreamBlocked(t, strings.Repeat(line, 40))

	cancel()
	drainClosed(t, ch, false)
}

func TestStream_cancelWhileBlocked_toolContent(t *testing.T) {
	line := "{\"message\":{\"content\":\"x\",\"tool_calls\":[{\"function\":{\"name\":\"t\",\"arguments\":{}}}]},\"done\":false}\n"
	ch, cancel := openStreamBlocked(t, strings.Repeat(line, 40))

	cancel()
	drainClosed(t, ch, false)
}

func TestStream_cancelWhileBlocked_toolOnly(t *testing.T) {
	line := "{\"message\":{\"content\":\"\",\"tool_calls\":[{\"function\":{\"name\":\"t\",\"arguments\":{}}}]},\"done\":false}\n"
	ch, cancel := openStreamBlocked(t, strings.Repeat(line, 40))

	cancel()
	drainClosed(t, ch, false)
}

func TestStream_cancelWhileBlocked_finalDone(t *testing.T) {
	tool := "{\"message\":{\"content\":\"\",\"tool_calls\":[{\"function\":{\"name\":\"t\",\"arguments\":{}}}]},\"done\":false}\n"
	last := "{\"message\":{\"content\":\"\",\"tool_calls\":[{\"function\":{\"name\":\"t\",\"arguments\":{}}}]},\"done\":true,\"eval_count\":1,\"prompt_eval_count\":2}\n"
	ch, cancel := openStreamBlocked(t, strings.Repeat(tool, 31)+last)

	cancel()
	drainClosed(t, ch, false)
}

func TestStream_cancelWhileBlocked_decodeError(t *testing.T) {
	line := "{\"message\":{\"content\":\"x\"},\"done\":false}\n"
	ch, cancel := openStreamBlocked(t, strings.Repeat(line, 32)+"{bad\n")

	cancel()
	drainClosed(t, ch, true)
}

func TestStream_cancelWhileBlocked_serverError(t *testing.T) {
	line := "{\"message\":{\"content\":\"x\"},\"done\":false}\n"
	ch, cancel := openStreamBlocked(t, strings.Repeat(line, 32)+"{\"error\":\"gone\"}\n")

	cancel()
	drainClosed(t, ch, true)
}

func TestTruncateForError(t *testing.T) {
	t.Parallel()

	if got := truncateForError([]byte("short")); got != "short" {
		t.Fatalf("got %q", got)
	}

	long := strings.Repeat("x", maxErrorBody+10)
	got := truncateForError([]byte(long))
	if len(got) != maxErrorBody+len("...(truncated)") || !strings.HasSuffix(got, "...(truncated)") {
		t.Fatalf("len = %d", len(got))
	}
}

func TestNormalizeMessages_empty(t *testing.T) {
	t.Parallel()

	if got := normalizeMessages(nil); len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestOllamaTools_empty_nil(t *testing.T) {
	t.Parallel()

	if got := ollamaTools(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}
}

func TestResponseFormat_branches(t *testing.T) {
	t.Parallel()

	if got := responseFormat(nil); got != nil {
		t.Fatalf("got %v, want nil", got)
	}

	if got := responseFormat(&ai.ResponseFormat{}); got != "json" {
		t.Fatalf("got %v, want json", got)
	}

	schema := map[string]any{"type": "object"}
	if got := responseFormat(&ai.ResponseFormat{JSONSchema: schema}); !equalMaps(got, schema) {
		t.Fatalf("got %v", got)
	}
}

func equalMaps(got any, want map[string]any) bool {
	m, ok := got.(map[string]any)
	if !ok || len(m) != len(want) {
		return false
	}

	for k, v := range want {
		if m[k] != v {
			return false
		}
	}

	return true
}
