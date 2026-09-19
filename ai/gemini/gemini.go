package gemini

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"google.golang.org/genai"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/codec"
	"github.com/zenta-dev/zever/internal/endpoint"
	"github.com/zenta-dev/zever/internal/httpclient"
)

// funcArgsCodec handles JSON encoding/decoding of function-call argument maps.
var funcArgsCodec = codec.JSONCodec[map[string]any]{}

// adapter implements ai.AI via Google Generative AI.
type adapter struct {
	client *genai.Client
	apiKey string
}

// newGenaiClient is overridden in tests to simulate NewClient failures.
var newGenaiClient = genai.NewClient //nolint:gochecknoglobals

// New creates an AI backed by Gemini.
//
// It validates opts via ai.Options.Validate and requires a non-empty APIKey.
func New(opts ai.Options) (ai.AI, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	if strings.TrimSpace(opts.APIKey) == "" {
		return nil, &ai.InvalidOptionsError{Reason: "api_key is required"}
	}

	httpClient := newHTTPClient(opts.Timeout)

	// For httptest TLS servers on loopback, allow insecure certs.
	// Only exact loopback hosts match: a substring check would wrongly
	// trust hosts like 127.0.0.1.evil.com.
	if tr, ok := httpClient.Transport.(*http.Transport); ok && tr.TLSClientConfig != nil {
		tr.TLSClientConfig.InsecureSkipVerify = loopbackBaseURL(opts.BaseURL)
	}

	cc := &genai.ClientConfig{
		APIKey:     opts.APIKey,
		Backend:    genai.BackendGeminiAPI,
		HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{
			BaseURL: opts.BaseURL,
		},
	}

	client, err := newGenaiClient(context.Background(), cc)
	if err != nil {
		return nil, mapAndRedact(err, opts.APIKey)
	}

	return &adapter{client: client, apiKey: opts.APIKey}, nil
}

// loopbackBaseURL reports whether rawURL parses to a loopback host
// ("localhost" or a loopback IP such as 127.0.0.1 or ::1).
func loopbackBaseURL(rawURL string) bool {
	return endpoint.IsLoopbackURL(rawURL)
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return httpclient.NewClient(timeout)
}

// Generate implements ai.AI.
func (a *adapter) Generate(ctx context.Context, model string, messages []ai.Message, opts ai.GenerateOptions) (ai.Generation, error) {
	if strings.TrimSpace(model) == "" {
		return ai.Generation{}, fmt.Errorf("%w: model is required", ai.ErrInvalidRequest)
	}

	contents, system := toGenaiContents(messages)
	cfg := buildGenerateConfig(opts, system)

	resp, err := a.client.Models.GenerateContent(ctx, model, contents, cfg)
	if err != nil {
		return ai.Generation{}, mapAndRedact(err, a.apiKey)
	}

	if resp == nil || len(resp.Candidates) == 0 {
		return ai.Generation{}, fmt.Errorf("%w: no candidates returned", ai.ErrNotSupported)
	}

	content, toolCalls := extractGeneration(resp)
	usage := ai.Usage{}
	if resp.UsageMetadata != nil {
		usage.PromptTokens = int(resp.UsageMetadata.PromptTokenCount)
		usage.CompletionTokens = int(resp.UsageMetadata.CandidatesTokenCount)
	}

	finish := ""
	if len(resp.Candidates) > 0 && resp.Candidates[0] != nil {
		finish = string(resp.Candidates[0].FinishReason)
	}

	return ai.Generation{
		Content:      content,
		ToolCalls:    toolCalls,
		Usage:        usage,
		FinishReason: finish,
	}, nil
}

// Stream implements ai.AI.
func (a *adapter) Stream(ctx context.Context, model string, messages []ai.Message, opts ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("%w: model is required", ai.ErrInvalidRequest)
	}

	contents, system := toGenaiContents(messages)
	cfg := buildGenerateConfig(opts, system)

	iter := a.client.Models.GenerateContentStream(ctx, model, contents, cfg)

	ch := make(chan ai.StreamChunk)

	go func() {
		defer close(ch)

		for resp, err := range iter {
			if err != nil {
				select {
				case ch <- ai.StreamChunk{Err: mapAndRedact(err, a.apiKey)}:
				case <-ctx.Done():
				}

				return
			}

			if resp == nil || len(resp.Candidates) == 0 {
				continue
			}

			for _, cand := range resp.Candidates {
				if cand == nil || cand.Content == nil {
					continue
				}

				var sbDelta strings.Builder
				for _, p := range cand.Content.Parts {
					if p.Text != "" {
						sbDelta.WriteString(p.Text)
					}

					if p.FunctionCall != nil {
						args, _ := funcArgsCodec.Encode(p.FunctionCall.Args) //nolint:errcheck

						select {
						case ch <- ai.StreamChunk{
							ToolCallID:    "",
							ToolName:      p.FunctionCall.Name,
							ToolArgsDelta: string(args),
						}:
						case <-ctx.Done():
							return
						}
					}
				}

				delta := sbDelta.String()
				if delta != "" {
					select {
					case ch <- ai.StreamChunk{Delta: delta}:
					case <-ctx.Done():
						return
					}
				}

				if cand.FinishReason != "" {
					select {
					case ch <- ai.StreamChunk{Done: true, FinishReason: string(cand.FinishReason)}:
					case <-ctx.Done():
						return
					}
				}

				if resp.UsageMetadata != nil {
					u := ai.Usage{
						PromptTokens:     int(resp.UsageMetadata.PromptTokenCount),
						CompletionTokens: int(resp.UsageMetadata.CandidatesTokenCount),
					}

					select {
					case ch <- ai.StreamChunk{Usage: &u}:
					case <-ctx.Done():
						return
					}
				}
			}
		}

		select {
		case ch <- ai.StreamChunk{Done: true}:
		case <-ctx.Done():
		}
	}()

	return ch, nil
}

// Embed implements ai.AI.
func (a *adapter) Embed(ctx context.Context, model string, inputs []string, opts ai.EmbedOptions) ([][]float32, error) {
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("%w: model is required", ai.ErrInvalidRequest)
	}

	if len(inputs) == 0 {
		return nil, fmt.Errorf("%w: inputs is required", ai.ErrInvalidRequest)
	}

	contents := make([]*genai.Content, 0, len(inputs))
	for _, in := range inputs {
		contents = append(contents, genai.NewContentFromText(in, genai.RoleUser))
	}

	cfg := &genai.EmbedContentConfig{}
	if opts.Dimensions > 0 {
		v := int32(opts.Dimensions) //nolint:gosec
		cfg.OutputDimensionality = &v
	}

	resp, err := a.client.Models.EmbedContent(ctx, model, contents, cfg)
	if err != nil {
		return nil, mapAndRedact(err, a.apiKey)
	}

	if resp == nil || len(resp.Embeddings) == 0 {
		return nil, fmt.Errorf("%w: no embeddings", ai.ErrNotSupported)
	}

	out := make([][]float32, len(resp.Embeddings))
	for i, emb := range resp.Embeddings {
		if emb == nil {
			out[i] = []float32{}
			continue
		}

		vals := emb.Values
		if opts.Dimensions > 0 && len(vals) > opts.Dimensions {
			vals = vals[:opts.Dimensions]
		}

		cp := make([]float32, len(vals))
		copy(cp, vals)
		out[i] = cp
	}

	return out, nil
}

// Close implements ai.AI.
func (a *adapter) Close() error {
	return nil
}

func buildGenerateConfig(opts ai.GenerateOptions, system *genai.Content) *genai.GenerateContentConfig {
	cfg := &genai.GenerateContentConfig{}
	if opts.Temperature != nil {
		cfg.Temperature = opts.Temperature
	}

	if opts.MaxTokens > 0 {
		cfg.MaxOutputTokens = int32(opts.MaxTokens) //nolint:gosec
	}

	if opts.TopP != nil {
		cfg.TopP = opts.TopP
	}

	if opts.ResponseFormat != nil {
		switch opts.ResponseFormat.Type {
		case "json_object":
			cfg.ResponseMIMEType = "application/json"
		case "json_schema":
			cfg.ResponseMIMEType = "application/json"

			if opts.ResponseFormat.JSONSchema != nil {
				cfg.ResponseJsonSchema = opts.ResponseFormat.JSONSchema
			}
		default:
			if opts.ResponseFormat.JSONSchema != nil {
				cfg.ResponseMIMEType = "application/json"
				cfg.ResponseJsonSchema = opts.ResponseFormat.JSONSchema
			} else if opts.ResponseFormat.Type != "" {
				// Map any non-empty type that looks like json to mime type.
				if strings.Contains(strings.ToLower(opts.ResponseFormat.Type), "json") {
					cfg.ResponseMIMEType = "application/json"
				}
			}
		}
	}

	if len(opts.Tools) > 0 {
		decls := make([]*genai.FunctionDeclaration, 0, len(opts.Tools))
		for _, t := range opts.Tools {
			decls = append(decls, &genai.FunctionDeclaration{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  convertMapToSchema(t.Parameters),
			})
		}

		cfg.Tools = []*genai.Tool{{FunctionDeclarations: decls}}

		switch opts.ToolChoice {
		case ai.ToolChoiceAuto:
			cfg.ToolConfig = &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAuto},
			}
		case ai.ToolChoiceRequired:
			cfg.ToolConfig = &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeAny},
			}
		case ai.ToolChoiceNone:
			cfg.ToolConfig = &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{Mode: genai.FunctionCallingConfigModeNone},
			}
		}
	}

	if system != nil {
		cfg.SystemInstruction = system
	}

	return cfg
}

func toGenaiContents(messages []ai.Message) ([]*genai.Content, *genai.Content) {
	var systemParts []*genai.Part

	contents := make([]*genai.Content, 0, len(messages))

	for _, m := range messages {
		switch m.Role { //nolint:exhaustive // default explicitly maps RoleUser and unknown roles to user content
		case ai.RoleSystem:
			if m.Content != "" {
				systemParts = append(systemParts, genai.NewPartFromText(m.Content))
			}
		case ai.RoleAssistant:
			parts := make([]*genai.Part, 0, 1+len(m.ToolCalls))
			if m.Content != "" {
				parts = append(parts, genai.NewPartFromText(m.Content))
			}

			for _, tc := range m.ToolCalls {
				var args map[string]any
				if tc.Arguments != "" {
					args, _ = funcArgsCodec.Decode([]byte(tc.Arguments))
				}

				if args == nil {
					args = map[string]any{}
				}

				parts = append(parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{Name: tc.Name, Args: args},
				})
			}

			if len(parts) == 0 {
				continue
			}

			contents = append(contents, &genai.Content{Role: string(genai.RoleModel), Parts: parts})
		case ai.RoleTool:
			resp := map[string]any{}
			if m.Content != "" {
				decoded, err := funcArgsCodec.Decode([]byte(m.Content))
				if err != nil {
					resp = map[string]any{"result": m.Content}
				} else {
					resp = decoded
				}
			}

			name := m.ToolCallID
			if name == "" {
				name = "tool"
			}

			contents = append(contents, &genai.Content{
				Role:  string(genai.RoleUser),
				Parts: []*genai.Part{{FunctionResponse: &genai.FunctionResponse{Name: name, Response: resp}}},
			})
		default:
			// User and unknown roles map to user.
			parts := []*genai.Part{}
			if m.Content != "" {
				parts = append(parts, genai.NewPartFromText(m.Content))
			} else {
				// Preserve empty user message as empty part to avoid empty content.
				parts = append(parts, genai.NewPartFromText(""))
			}

			contents = append(contents, &genai.Content{Role: string(genai.RoleUser), Parts: parts})
		}
	}

	var system *genai.Content
	if len(systemParts) > 0 {
		system = &genai.Content{Parts: systemParts}
	}

	return contents, system
}

func extractGeneration(resp *genai.GenerateContentResponse) (string, []ai.ToolCall) {
	var sb strings.Builder

	var toolCalls []ai.ToolCall

	for _, cand := range resp.Candidates {
		if cand == nil || cand.Content == nil {
			continue
		}

		for _, p := range cand.Content.Parts {
			if p.Text != "" {
				sb.WriteString(p.Text)
			}

			if p.FunctionCall != nil {
				argsJSON, err := funcArgsCodec.Encode(p.FunctionCall.Args)
				if err != nil || p.FunctionCall.Args == nil {
					argsJSON = []byte("{}")
				}

				toolCalls = append(toolCalls, ai.ToolCall{
					ID:        p.FunctionCall.Name,
					Name:      p.FunctionCall.Name,
					Arguments: string(argsJSON),
				})
			}
		}
	}

	return sb.String(), toolCalls
}

func convertMapToSchema(m map[string]any) *genai.Schema {
	if m == nil {
		return nil
	}

	s := &genai.Schema{}

	if t, ok := m["type"].(string); ok {
		switch strings.ToLower(t) {
		case "string":
			s.Type = genai.TypeString
		case "number":
			s.Type = genai.TypeNumber
		case "integer":
			s.Type = genai.TypeInteger
		case "boolean":
			s.Type = genai.TypeBoolean
		case "array":
			s.Type = genai.TypeArray
		case "object":
			s.Type = genai.TypeObject
		}
	}

	if v, ok := m["description"].(string); ok {
		s.Description = v
	}

	if v, ok := m["format"].(string); ok {
		s.Format = v
	}

	if v, ok := m["title"].(string); ok {
		s.Title = v
	}

	if v, ok := m["nullable"].(bool); ok {
		s.Nullable = genai.Ptr(v)
	}

	if props, ok := m["properties"].(map[string]any); ok {
		s.Properties = make(map[string]*genai.Schema, len(props))
		for k, val := range props {
			if sub, ok := val.(map[string]any); ok {
				s.Properties[k] = convertMapToSchema(sub)
			}
		}
	}

	if req, ok := m["required"].([]any); ok {
		for _, r := range req {
			if rs, ok := r.(string); ok {
				s.Required = append(s.Required, rs)
			}
		}
	} else if req, ok := m["required"].([]string); ok {
		s.Required = append(s.Required, req...)
	}

	if enumVals, ok := m["enum"].([]any); ok {
		for _, e := range enumVals {
			if es, ok := e.(string); ok {
				s.Enum = append(s.Enum, es)
			}
		}
	} else if enumVals, ok := m["enum"].([]string); ok {
		s.Enum = append(s.Enum, enumVals...)
	}

	if items, ok := m["items"].(map[string]any); ok {
		s.Items = convertMapToSchema(items)
	}

	return s
}

func mapAndRedact(err error, apiKey string) error {
	if err == nil {
		return nil
	}

	mapped := mapError(err)
	msg := mapped.Error()

	if apiKey != "" && strings.Contains(msg, apiKey) {
		msg = strings.ReplaceAll(msg, apiKey, "REDACTED")
	}

	// Also redact header name value pattern case-insensitively.
	msg = strings.ReplaceAll(msg, "x-goog-api-key", "REDACTED")
	msg = strings.ReplaceAll(msg, "X-Goog-Api-Key", "REDACTED")
	msg = strings.ReplaceAll(msg, "X-GOOG-API-KEY", "REDACTED")

	if msg == mapped.Error() {
		return mapped
	}

	// Preserve sentinel wrapping if mapped is sentinel-based.
	if errors.Is(mapped, ai.ErrAuth) {
		return fmt.Errorf("%w: %s", ai.ErrAuth, msg)
	}

	if errors.Is(mapped, ai.ErrRateLimited) {
		return &ai.RateLimitedError{}
	}

	if errors.Is(mapped, ai.ErrInvalidRequest) {
		return fmt.Errorf("%w: %s", ai.ErrInvalidRequest, msg)
	}

	if errors.Is(mapped, ai.ErrModelNotFound) {
		return fmt.Errorf("%w: %s", ai.ErrModelNotFound, msg)
	}

	if errors.Is(mapped, ai.ErrNotSupported) {
		return fmt.Errorf("%w: %s", ai.ErrNotSupported, msg)
	}

	// Generic: return new error with redacted message, wrapping original if possible.
	return errors.New(msg)
}

func mapError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr genai.APIError
	if errors.As(err, &apiErr) {
		return mapAPIError(apiErr)
	}

	var apiErrPtr *genai.APIError
	if errors.As(err, &apiErrPtr) {
		return mapAPIError(*apiErrPtr)
	}

	// Fallback string inspection for status codes embedded in message.
	msg := err.Error()
	switch {
	case strings.Contains(msg, "401") || strings.Contains(msg, "403") || strings.Contains(strings.ToLower(msg), "unauthenticated") || strings.Contains(strings.ToLower(msg), "permission_denied"):
		return fmt.Errorf("%w: %s", ai.ErrAuth, msg)
	case strings.Contains(msg, "429") || strings.Contains(strings.ToLower(msg), "rate limit") || strings.Contains(strings.ToLower(msg), "resource_exhausted"):
		return &ai.RateLimitedError{}
	case strings.Contains(msg, "400") || strings.Contains(strings.ToLower(msg), "invalid_argument"):
		return fmt.Errorf("%w: %s", ai.ErrInvalidRequest, msg)
	case strings.Contains(msg, "404") || strings.Contains(strings.ToLower(msg), "not_found"):
		return fmt.Errorf("%w: %s", ai.ErrModelNotFound, msg)
	}

	return err
}

func mapAPIError(e genai.APIError) error {
	switch e.Code {
	case 401, 403:
		return fmt.Errorf("%w: %s", ai.ErrAuth, e.Message)
	case 429:
		if e.Message != "" {
			return fmt.Errorf("%w: %s", &ai.RateLimitedError{}, e.Message)
		}

		return &ai.RateLimitedError{}
	case 400:
		return fmt.Errorf("%w: %s", ai.ErrInvalidRequest, e.Message)
	case 404:
		return fmt.Errorf("%w: %s", ai.ErrModelNotFound, e.Message)
	default:
		if e.Message != "" {
			return errors.New(e.Message)
		}

		return errors.New(e.Error())
	}
}

var _ = errors.Is
