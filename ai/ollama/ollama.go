package ollama

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/internal/endpoint"
	"github.com/zenta-dev/zever/internal/httpclient"
)

const (
	maxResponseBytes = 16 << 20
	maxErrorBody     = 1024
)

// adapter talks to an Ollama server over plain HTTP with whole-exchange timeouts.
type adapter struct {
	addr         string
	defaultModel string
	client       *http.Client
}

// New creates an AI backed by the Ollama server at opts.Addr.
func New(opts Options) (ai.AI, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	addr := opts.Addr
	if addr == "" {
		addr = DefaultAddr
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	transport := opts.Transport
	if transport == nil {
		return &adapter{
			addr:         addr,
			defaultModel: opts.Model,
			client:       httpclient.NewClient(timeout),
		}, nil
	}

	return &adapter{
		addr:         addr,
		defaultModel: opts.Model,
		client:       &http.Client{Timeout: timeout, Transport: transport},
	}, nil
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   any           `json:"format,omitempty"`
	Options  *chatOptions  `json:"options,omitempty"`
	Tools    []ollamaTool  `json:"tools,omitempty"`
}

type ollamaTool struct {
	Type     string         `json:"type"`
	Function ollamaFunction `json:"function"`
}

type ollamaFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ollamaToolCall struct {
	Function struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	} `json:"function"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatOptions struct {
	Temperature *float32 `json:"temperature,omitempty"`
	NumPredict  int      `json:"num_predict,omitempty"`
	TopP        *float32 `json:"top_p,omitempty"`
}

type chatResponse struct {
	Message struct {
		Content   string           `json:"content"`
		ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	EvalCount       int `json:"eval_count"`
	PromptEvalCount int `json:"prompt_eval_count"`
}

type embedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

var (
	// requestCodec encodes outgoing Ollama request bodies. It is kept generic
	// over any (rather than a per-request-type codec) because encode below
	// must accept arbitrary values, including ones that are not valid JSON,
	// to preserve its existing error-mapping behavior.
	requestCodec        = codec.JSONCodec[any]{}
	chatResponseCodec   = codec.JSONCodec[chatResponse]{}
	streamResponseCodec = codec.JSONCodec[streamResponse]{}
	embedResponseCodec  = codec.JSONCodec[embedResponse]{}
	toolArgsCodec       = codec.JSONCodec[map[string]any]{}
)

// encode marshals v for an Ollama request.
func encode(v any) ([]byte, error) {
	body, err := requestCodec.Encode(v)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	return body, nil
}

// Generate produces one chat completion, mapping roles, options, tools, and JSON formats.
func (a *adapter) Generate(ctx context.Context, model string, messages []ai.Message, opts ai.GenerateOptions) (ai.Generation, error) {
	if model == "" {
		model = a.defaultModel
	}

	if model == "" {
		return ai.Generation{}, ErrNoModel
	}

	req := chatRequest{
		Model:    model,
		Messages: normalizeMessages(messages),
		Stream:   false,
		Format:   responseFormat(opts.ResponseFormat),
		Options:  chatOpts(opts),
		Tools:    ollamaTools(opts.Tools),
	}

	body, err := encode(req)
	if err != nil {
		return ai.Generation{}, err
	}

	respBody, err := a.doPost(ctx, "/api/chat", body)
	if err != nil {
		return ai.Generation{}, err
	}

	chatResp, unmarshalErr := chatResponseCodec.Decode(respBody)
	if unmarshalErr != nil {
		return ai.Generation{}, fmt.Errorf("ollama: unmarshal response: %w", unmarshalErr)
	}

	toolCalls, err := toolCalls(chatResp.Message.ToolCalls)
	if err != nil {
		return ai.Generation{}, err
	}

	return ai.Generation{
		Content:   chatResp.Message.Content,
		ToolCalls: toolCalls,
		Usage: ai.Usage{
			PromptTokens:     chatResp.PromptEvalCount,
			CompletionTokens: chatResp.EvalCount,
		},
	}, nil
}

type streamResponse struct {
	Message struct {
		Content   string           `json:"content"`
		ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
	} `json:"message"`
	Done            bool   `json:"done"`
	EvalCount       int    `json:"eval_count"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	Error           string `json:"error,omitempty"`
}

// Stream opens a newline-delimited JSON completion stream.
// The caller drains the channel; Done carries final usage.
func (a *adapter) Stream(ctx context.Context, model string, messages []ai.Message, opts ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	if model == "" {
		model = a.defaultModel
	}

	if model == "" {
		return nil, ErrNoModel
	}

	req := chatRequest{
		Model:    model,
		Messages: normalizeMessages(messages),
		Stream:   true,
		Format:   responseFormat(opts.ResponseFormat),
		Options:  chatOpts(opts),
		Tools:    ollamaTools(opts.Tools),
	}

	body, err := encode(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := postRequest(ctx, a.addr, "/api/chat", body)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()

		b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))

		return nil, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, string(b))
	}

	ch := make(chan ai.StreamChunk, 32)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		// codec.Codec[V] has no streaming form (only Decode([]byte) (V, error)),
		// so the full response is buffered up front and then split into its
		// newline-delimited JSON records. This trades incremental decoding as
		// bytes arrive for a single buffered read; acceptable for the response
		// sizes these single API calls produce.
		respBody, readErr := io.ReadAll(resp.Body)
		if readErr != nil {
			select {
			case ch <- ai.StreamChunk{Err: fmt.Errorf("ollama: read stream: %w", readErr)}:
			case <-ctx.Done():
			}

			return
		}

		lines := bytes.Split(respBody, []byte("\n"))

		for _, line := range lines {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}

			sr, err := streamResponseCodec.Decode(line)
			if err != nil {
				select {
				case ch <- ai.StreamChunk{Err: fmt.Errorf("ollama: decode stream: %w", err)}:
				case <-ctx.Done():
				}

				return
			}

			if sr.Error != "" {
				select {
				case ch <- ai.StreamChunk{Err: fmt.Errorf("ollama: stream error: %s", sr.Error)}:
				case <-ctx.Done():
				}

				return
			}

			if len(sr.Message.ToolCalls) > 0 {
				if sr.Message.Content != "" {
					select {
					case ch <- ai.StreamChunk{Delta: sr.Message.Content}:
					case <-ctx.Done():
						return
					}
				}

				for i, tc := range sr.Message.ToolCalls {
					argsBytes, err := toolArgsCodec.Encode(tc.Function.Arguments)
					if err != nil {
						select {
						case ch <- ai.StreamChunk{Err: fmt.Errorf("ollama: marshal tool arguments: %w", err)}:
						case <-ctx.Done():
						}

						return
					}

					argsStr := string(argsBytes)
					if tc.Function.Arguments == nil {
						argsStr = "{}"
					}

					select {
					case ch <- ai.StreamChunk{
						ToolCallID:    fmt.Sprintf("call_%d", i),
						ToolName:      tc.Function.Name,
						ToolArgsDelta: argsStr,
					}:
					case <-ctx.Done():
						return
					}
				}

				if sr.Done {
					usage := ai.Usage{PromptTokens: sr.PromptEvalCount, CompletionTokens: sr.EvalCount}
					select {
					case ch <- ai.StreamChunk{Done: true, Usage: &usage}:
					case <-ctx.Done():
					}

					return
				}
			} else {
				chunk := ai.StreamChunk{Delta: sr.Message.Content, Done: sr.Done}
				if sr.Done {
					chunk.Usage = &ai.Usage{PromptTokens: sr.PromptEvalCount, CompletionTokens: sr.EvalCount}
				}

				select {
				case ch <- chunk:
				case <-ctx.Done():
					return
				}

				if sr.Done {
					return
				}
			}
		}
	}()

	return ch, nil
}

// Embed returns one vector per input. Dimensions requests are not supported.
func (a *adapter) Embed(ctx context.Context, model string, inputs []string, opts ai.EmbedOptions) ([][]float32, error) {
	if opts.Dimensions > 0 {
		return nil, fmt.Errorf("ollama: embed: %w", ai.ErrNotSupported)
	}

	if model == "" {
		model = a.defaultModel
	}

	if model == "" {
		return nil, ErrNoModel
	}

	body, err := encode(embedRequest{Model: model, Input: inputs})
	if err != nil {
		return nil, err
	}

	respBody, err := a.doPost(ctx, "/api/embed", body)
	if err != nil {
		return nil, fmt.Errorf("ollama: embed %w", err)
	}

	embedResp, err := embedResponseCodec.Decode(respBody)
	if err != nil {
		return nil, fmt.Errorf("ollama: unmarshal embed response: %w", err)
	}

	return embedResp.Embeddings, nil
}

// Close releases no resources and always succeeds.
func (a *adapter) Close() error {
	return nil
}

// postRequest builds a JSON POST for addr+path, rejecting endpoint shapes
// that fail parsing or violate the http(s)+host+no-userinfo policy.
// Address shape is also enforced at Open; this re-checks per request
// because the address is used verbatim on every exchange.
func postRequest(ctx context.Context, addr, path string, body []byte) (*http.Request, error) {
	endpoint := strings.TrimRight(addr, "/") + path

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama: create request: %w", err)
	}

	if err := checkEndpoint(req.URL); err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	return req, nil
}

// checkEndpoint enforces the http(s)+host+no-userinfo policy on a parsed URL.
func checkEndpoint(u *url.URL) error {
	if _, err := endpoint.ValidateURL(u.String(),
		endpoint.WithAllowInsecure(true),
		endpoint.WithRejectUserinfo(),
	); err != nil {
		return fmt.Errorf("ollama: url %q %s", u.String(), addrReason(err))
	}

	return nil
}

// doPost sends body to path and returns the bounded response bytes.
func (a *adapter) doPost(ctx context.Context, path string, body []byte) ([]byte, error) {
	httpReq, err := postRequest(ctx, a.addr, path, body)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: request failed: %w", err)
	}

	defer func() { _ = resp.Body.Close() }()

	respBody, err := httpclient.ReadLimited(resp.Body, maxResponseBytes)
	if err != nil {
		if errors.Is(err, httpclient.ErrTooLarge) {
			return nil, fmt.Errorf("ollama: response exceeds %d bytes", maxResponseBytes)
		}
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		snippet := truncateForError(respBody)

		return nil, fmt.Errorf("ollama: status %d: %s", resp.StatusCode, snippet)
	}

	return respBody, nil
}

// normalizeMessages keeps system/assistant roles and coerces the rest to user.
func normalizeMessages(messages []ai.Message) []chatMessage {
	out := make([]chatMessage, len(messages))
	for i, m := range messages {
		role := m.Role
		if role != ai.RoleSystem && role != ai.RoleAssistant {
			role = ai.RoleUser
		}

		out[i] = chatMessage{Role: string(role), Content: m.Content}
	}

	return out
}

// chatOpts maps sampling knobs, returning nil when none are set.
func chatOpts(opts ai.GenerateOptions) *chatOptions {
	if opts.Temperature != nil || opts.MaxTokens > 0 || opts.TopP != nil {
		return &chatOptions{
			Temperature: opts.Temperature,
			NumPredict:  opts.MaxTokens,
			TopP:        opts.TopP,
		}
	}

	return nil
}

// ollamaTools maps tool definitions to Ollama function tools.
func ollamaTools(tools []ai.Tool) []ollamaTool {
	if len(tools) == 0 {
		return nil
	}

	out := make([]ollamaTool, len(tools))
	for i, t := range tools {
		out[i] = ollamaTool{
			Type: "function",
			Function: ollamaFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	return out
}

// responseFormat maps structured-output requests to Ollama formats:
// a schema map when one is present, "json" otherwise, nil when unset.
func responseFormat(rf *ai.ResponseFormat) any {
	if rf == nil {
		return nil
	}

	if len(rf.JSONSchema) == 0 {
		return "json"
	}

	if s, ok := rf.JSONSchema["schema"]; ok {
		if m, ok := s.(map[string]any); ok {
			return m
		}
	}

	return rf.JSONSchema
}

// toolCalls assigns deterministic call_N IDs to model tool invocations.
func toolCalls(calls []ollamaToolCall) ([]ai.ToolCall, error) {
	out := make([]ai.ToolCall, 0, len(calls))
	for i, tc := range calls {
		argsBytes, err := toolArgsCodec.Encode(tc.Function.Arguments)
		if err != nil {
			return nil, fmt.Errorf("ollama: marshal tool arguments: %w", err)
		}

		argsStr := string(argsBytes)
		if tc.Function.Arguments == nil {
			argsStr = "{}"
		}

		out = append(out, ai.ToolCall{
			ID:        fmt.Sprintf("call_%d", i),
			Name:      tc.Function.Name,
			Arguments: argsStr,
		})
	}

	return out, nil
}

// truncateForError caps error bodies for safe inclusion in errors.
func truncateForError(body []byte) string {
	if len(body) <= maxErrorBody {
		return string(body)
	}

	return string(body[:maxErrorBody]) + "...(truncated)"
}

// requestTimeout reports the client timeout for tests.
func (a *adapter) requestTimeout() time.Duration {
	return a.client.Timeout
}
