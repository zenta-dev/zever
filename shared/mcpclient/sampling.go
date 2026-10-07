package mcpclient

import "context"

// Sampler answers a server sampling/create request with a model completion.
// It must not call Client methods: hooks run while a call holds the
// transport lock, so re-entry would deadlock.
type Sampler func(ctx context.Context, req SampleRequest) (SampleResult, error)

// SampleRequest carries a server sampling request. Payloads stay untyped:
// message shapes vary by negotiated protocol version.
type SampleRequest struct {
	// Messages are the conversation turns to complete.
	Messages []map[string]any
	// Params carries the remaining sampling parameters.
	Params map[string]any
}

// SampleResult carries the model completion back to the server.
type SampleResult struct {
	// Content is the result object (typically a content block).
	Content map[string]any
}

// ElicitAction answers a server elicitation.
type ElicitAction string

// Elicitation answers.
const (
	// ElicitAccept approves the proposed action.
	ElicitAccept ElicitAction = "accept"
	// ElicitDecline rejects the proposed action.
	ElicitDecline ElicitAction = "decline"
	// ElicitCancel aborts the workflow.
	ElicitCancel ElicitAction = "cancel"
)

// ElicitRequest carries a server elicitation.
type ElicitRequest struct {
	// Message is the human-readable proposal.
	Message string
	// Schema constrains the answer content; nil means free-form.
	Schema map[string]any
}

// ElicitPolicy answers elicitations. A nil policy auto-declines. Like
// Sampler, it must not call Client methods.
type ElicitPolicy func(ctx context.Context, req ElicitRequest) (ElicitAction, map[string]any, error)

// Option configures a Client.
type Option func(*Client)

// WithSampler handles server sampling/create requests.
func WithSampler(s Sampler) Option {
	return func(c *Client) { c.onSampling = s }
}

// WithElicitPolicy answers server elicitation/create requests.
func WithElicitPolicy(p ElicitPolicy) Option {
	return func(c *Client) { c.onElicit = p }
}
