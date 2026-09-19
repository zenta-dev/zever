//nolint:dupl
package openai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"

	"github.com/zenta-dev/zever/ai"
	"github.com/zenta-dev/zever/internal/endpoint"
	"github.com/zenta-dev/zever/internal/httpclient"
	"github.com/zenta-dev/zever/internal/retry"
)

type adapter struct {
	client *openai.Client
	model  string
}

// New creates an AI backed by OpenAI.
func New(opts ai.Options) (ai.AI, error) {
	if err := validateOptions(opts); err != nil {
		return nil, err
	}

	if opts.APIKey == "" {
		return nil, &ai.InvalidOptionsError{Reason: "api_key is required"}
	}

	clientOpts := []option.RequestOption{
		option.WithAPIKey(opts.APIKey),
		option.WithHTTPClient(newHTTPClient()),
	}

	if opts.BaseURL != "" {
		clientOpts = append(clientOpts, option.WithBaseURL(opts.BaseURL))
	}

	client := openai.NewClient(clientOpts...)

	return &adapter{
		client: &client,
		model:  opts.Model,
	}, nil
}

func newHTTPClientFromTransport(tr *http.Transport) *http.Client {
	if tr == nil {
		return httpclient.NewClient(0)
	}

	return httpclient.NewClient(0, httpclient.WithTransport(tr.Clone()))
}

//go:noinline
func newHTTPClientImpl() *http.Client {
	if tr, ok := http.DefaultTransport.(*http.Transport); ok {
		return newHTTPClientFromTransport(tr)
	}

	return newHTTPClientFromTransport(nil)
}

var newHTTPClient = newHTTPClientImpl

//nolint:gocyclo
func (a *adapter) Generate(ctx context.Context, model string, messages []ai.Message, opts ai.GenerateOptions) (ai.Generation, error) {
	if model == "" {
		model = a.model
	}

	if model == "" {
		return ai.Generation{}, errors.New("openai: model is required")
	}

	if len(opts.Tools) > 0 && opts.ResponseFormat != nil {
		return ai.Generation{}, fmt.Errorf("openai: cannot set both response_format and tools: %w", ai.ErrNotSupported)
	}

	msgs := buildMessages(messages)

	params := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: msgs,
	}

	if opts.Temperature != nil {
		params.Temperature = openai.Float(float64(*opts.Temperature))
	}

	if opts.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(opts.MaxTokens))
	}

	if opts.TopP != nil {
		params.TopP = openai.Float(float64(*opts.TopP))
	}

	if opts.ResponseFormat != nil {
		applyResponseFormat(&params, opts.ResponseFormat)
	}

	if len(opts.Tools) > 0 {
		params.Tools = buildTools(opts.Tools)
	}

	if opts.ToolChoice != "" {
		params.ToolChoice = buildToolChoice(string(opts.ToolChoice))
	}

	if opts.ParallelToolCalls != nil {
		params.ParallelToolCalls = openai.Bool(*opts.ParallelToolCalls)
	}

	resp, err := a.client.Chat.Completions.New(ctx, params)
	if err != nil {
		return ai.Generation{}, fmt.Errorf("openai: generate: %w", mapError(err))
	}

	if resp == nil || len(resp.Choices) == 0 {
		return ai.Generation{}, errors.New("openai: no choices returned")
	}

	promptTokens, err := safeInt64ToInt(resp.Usage.PromptTokens)
	if err != nil {
		return ai.Generation{}, fmt.Errorf("openai: prompt_tokens overflow: %w", err)
	}

	completionTokens, err := safeInt64ToInt(resp.Usage.CompletionTokens)
	if err != nil {
		return ai.Generation{}, fmt.Errorf("openai: completion_tokens overflow: %w", err)
	}

	var toolCalls []ai.ToolCall
	if len(resp.Choices[0].Message.ToolCalls) > 0 {
		toolCalls = make([]ai.ToolCall, len(resp.Choices[0].Message.ToolCalls))
		for i, tc := range resp.Choices[0].Message.ToolCalls {
			toolCalls[i] = ai.ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			}
		}
	}

	return ai.Generation{
		Content:      resp.Choices[0].Message.Content,
		ToolCalls:    toolCalls,
		FinishReason: resp.Choices[0].FinishReason,
		Usage: ai.Usage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
		},
	}, nil
}

func (a *adapter) Embed(ctx context.Context, model string, inputs []string, opts ai.EmbedOptions) ([][]float32, error) {
	if model == "" {
		model = a.model
	}

	if model == "" {
		return nil, errors.New("openai: model is required")
	}

	params := openai.EmbeddingNewParams{
		Model: model,
		Input: openai.EmbeddingNewParamsInputUnion{
			OfArrayOfStrings: inputs,
		},
	}

	if opts.Dimensions > 0 {
		params.Dimensions = openai.Int(int64(opts.Dimensions))
	}

	resp, err := a.client.Embeddings.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("openai: embed: %w", mapError(err))
	}

	result := make([][]float32, len(resp.Data))
	for i, d := range resp.Data {
		vec := make([]float32, len(d.Embedding))
		for j, v := range d.Embedding {
			vec[j] = float32(v)
		}

		result[i] = vec
	}

	return result, nil
}

//nolint:gocyclo
func (a *adapter) Stream(ctx context.Context, model string, messages []ai.Message, opts ai.GenerateOptions) (<-chan ai.StreamChunk, error) {
	if model == "" {
		model = a.model
	}

	if model == "" {
		return nil, errors.New("openai: model is required")
	}

	if len(opts.Tools) > 0 && opts.ResponseFormat != nil {
		return nil, fmt.Errorf("openai: cannot set both response_format and tools: %w", ai.ErrNotSupported)
	}

	msgs := buildMessages(messages)

	params := openai.ChatCompletionNewParams{
		Model:    model,
		Messages: msgs,
	}

	if opts.Temperature != nil {
		params.Temperature = openai.Float(float64(*opts.Temperature))
	}

	if opts.MaxTokens > 0 {
		params.MaxTokens = openai.Int(int64(opts.MaxTokens))
	}

	if opts.TopP != nil {
		params.TopP = openai.Float(float64(*opts.TopP))
	}

	if opts.ResponseFormat != nil {
		applyResponseFormat(&params, opts.ResponseFormat)
	}

	if len(opts.Tools) > 0 {
		params.Tools = buildTools(opts.Tools)
	}

	if opts.ToolChoice != "" {
		params.ToolChoice = buildToolChoice(string(opts.ToolChoice))
	}

	if opts.ParallelToolCalls != nil {
		params.ParallelToolCalls = openai.Bool(*opts.ParallelToolCalls)
	}

	stream := a.client.Chat.Completions.NewStreaming(ctx, params)

	ch := make(chan ai.StreamChunk)
	go func() {
		defer close(ch)
		defer stream.Close()

		var finishReason string

		for stream.Next() {
			chunk := stream.Current()
			if len(chunk.Choices) == 0 {
				continue
			}

			choice := chunk.Choices[0]
			if choice.FinishReason != "" {
				finishReason = choice.FinishReason
			}

			delta := choice.Delta
			if delta.Content != "" {
				select {
				case ch <- ai.StreamChunk{Delta: delta.Content}:
				case <-ctx.Done():
					return
				}
			}

			for _, tc := range delta.ToolCalls {
				if tc.ID == "" && tc.Function.Name == "" && tc.Function.Arguments == "" {
					continue
				}

				select {
				case ch <- ai.StreamChunk{
					ToolCallID:    tc.ID,
					ToolName:      tc.Function.Name,
					ToolArgsDelta: tc.Function.Arguments,
				}:
				case <-ctx.Done():
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			ch <- ai.StreamChunk{Err: fmt.Errorf("openai: stream: %w", mapError(err))}

			return
		}

		ch <- ai.StreamChunk{Done: true, FinishReason: finishReason}
	}()

	return ch, nil
}

func (a *adapter) Close() error {
	return nil
}

//nolint:dupl

func buildMessages(messages []ai.Message) []openai.ChatCompletionMessageParamUnion {
	msgs := make([]openai.ChatCompletionMessageParamUnion, len(messages))
	for i, m := range messages {
		switch m.Role {
		case ai.RoleSystem:
			msgs[i] = openai.SystemMessage(m.Content)
		case ai.RoleAssistant:
			if len(m.ToolCalls) > 0 {
				var asc openai.ChatCompletionAssistantMessageParam
				if m.Content != "" {
					asc.Content.OfString = openai.String(m.Content)
				}

				asc.ToolCalls = make([]openai.ChatCompletionMessageToolCallUnionParam, len(m.ToolCalls))
				for j, tc := range m.ToolCalls {
					asc.ToolCalls[j].OfFunction = &openai.ChatCompletionMessageFunctionToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageFunctionToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					}
				}

				msgs[i] = openai.ChatCompletionMessageParamUnion{OfAssistant: &asc}
			} else {
				msgs[i] = openai.AssistantMessage(m.Content)
			}
		case ai.RoleTool:
			msgs[i] = openai.ToolMessage(m.Content, m.ToolCallID)
		case ai.RoleUser:
			msgs[i] = openai.UserMessage(m.Content)
		default:
			msgs[i] = openai.UserMessage(m.Content)
		}
	}

	return msgs
}

func buildTools(tools []ai.Tool) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, len(tools))
	for i, t := range tools {
		out[i].OfFunction = &openai.ChatCompletionFunctionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name: t.Name,
			},
		}
		if t.Description != "" {
			out[i].OfFunction.Function.Description = openai.String(t.Description)
		}

		if t.Parameters != nil {
			out[i].OfFunction.Function.Parameters = shared.FunctionParameters(t.Parameters)
		}
	}

	return out
}

func buildToolChoice(choice string) openai.ChatCompletionToolChoiceOptionUnionParam {
	switch choice {
	case string(ai.ToolChoiceAuto):
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("auto")}
	case string(ai.ToolChoiceRequired):
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("required")}
	case string(ai.ToolChoiceNone):
		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String("none")}
	default:
		if strings.HasPrefix(choice, "tool:") {
			name := strings.TrimPrefix(choice, "tool:")
			return openai.ChatCompletionToolChoiceOptionUnionParam{
				OfFunctionToolChoice: &openai.ChatCompletionNamedToolChoiceParam{
					Function: openai.ChatCompletionNamedToolChoiceFunctionParam{Name: name},
				},
			}
		}

		return openai.ChatCompletionToolChoiceOptionUnionParam{OfAuto: openai.String(choice)}
	}
}

func applyResponseFormat(params *openai.ChatCompletionNewParams, rf *ai.ResponseFormat) {
	if len(rf.JSONSchema) > 0 {
		name := "response"
		if n, ok := rf.JSONSchema["name"].(string); ok && n != "" {
			name = n
		}

		var schema map[string]any

		if s, ok := rf.JSONSchema["schema"]; ok {
			if m, ok := s.(map[string]any); ok {
				schema = m
			}
		}

		if schema == nil {
			schema = make(map[string]any)
			for k, v := range rf.JSONSchema {
				if k == "name" || k == "strict" {
					continue
				}

				schema[k] = v
			}

			if len(schema) == 0 {
				schema = rf.JSONSchema
			}
		}

		strict := false
		if sv, ok := rf.JSONSchema["strict"].(bool); ok {
			strict = sv
		}

		params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
			OfJSONSchema: &openai.ResponseFormatJSONSchemaParam{
				JSONSchema: openai.ResponseFormatJSONSchemaJSONSchemaParam{
					Name:   name,
					Schema: schema,
					Strict: openai.Bool(strict),
				},
			},
		}

		return
	}

	params.ResponseFormat = openai.ChatCompletionNewParamsResponseFormatUnion{
		OfJSONObject: &openai.ResponseFormatJSONObjectParam{},
	}
}

func safeInt64ToInt(v int64) (int, error) {
	// Use 32-bit limits to ensure overflow is testable on 64-bit platforms.
	if v > int64(math.MaxInt32) || v < int64(math.MinInt32) {
		return 0, fmt.Errorf("value %d out of int range", v)
	}

	return int(v), nil
}

func mapError(err error) error {
	if err == nil {
		return nil
	}

	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return fmt.Errorf("%w: %s", ai.ErrAuth, apiErr.Message)
		case http.StatusTooManyRequests:
			retryAfter := parseRetryAfter(apiErr.Response)
			return &ai.RateLimitedError{RetryAfter: retryAfter}
		case http.StatusBadRequest:
			return fmt.Errorf("%w: %s", ai.ErrInvalidRequest, apiErr.Message)
		}
	}

	return err
}

func parseRetryAfter(resp *http.Response) time.Duration {
	if resp == nil {
		return 0
	}

	d, ok := retry.ParseRetryAfter(resp.Header.Get("Retry-After"), time.Now())
	if !ok {
		return 0
	}

	return d
}

func validateOptions(opts ai.Options) error {
	var errs []error

	if opts.Timeout < 0 {
		errs = append(errs, &ai.InvalidOptionsError{Reason: "timeout must be >= 0"})
	}

	if opts.BaseURL != "" {
		// Allow http for loopback in tests.
		if _, err := endpoint.ValidateURL(opts.BaseURL, endpoint.WithAllowLoopbackHTTP()); err != nil {
			errs = append(errs, &ai.InvalidOptionsError{Reason: baseURLReason(err)})
		}
	}

	return errors.Join(errs...)
}

// baseURLReason maps endpoint validation failures to the historical
// base_url reason strings.
func baseURLReason(err error) string {
	switch {
	case errors.Is(err, endpoint.ErrParse):
		return "base_url must be a valid URL"
	case errors.Is(err, endpoint.ErrNoScheme):
		return "base_url must include scheme"
	case errors.Is(err, endpoint.ErrNoHost):
		return "base_url must include host"
	default:
		return "base_url must use https scheme"
	}
}
