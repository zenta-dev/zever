package ai

// Role identifies the speaker in a conversation turn.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Message is a single turn in a conversation.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []ToolCall
	ToolCallID string
}

// Tool describes a callable function the model may invoke.
type Tool struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is a model-initiated invocation of a Tool.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

// ToolChoice controls how the model selects tools.
type ToolChoice string

const (
	ToolChoiceAuto     ToolChoice = "auto"
	ToolChoiceRequired ToolChoice = "required"
	ToolChoiceNone     ToolChoice = "none"
)

// ResponseFormat constrains model output formatting.
type ResponseFormat struct {
	Type       string
	JSONSchema map[string]any
}

// GenerateOptions configures a single generation request.
type GenerateOptions struct {
	Temperature       *float32
	MaxTokens         int
	TopP              *float32
	ResponseFormat    *ResponseFormat
	Tools             []Tool
	ToolChoice        ToolChoice
	ParallelToolCalls *bool
}

// Generation is the result of a Generate call.
type Generation struct {
	Content      string
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason string
}

// Usage reports token consumption.
type Usage struct {
	PromptTokens     int
	CompletionTokens int
}

// EmbedOptions configures embedding requests.
type EmbedOptions struct {
	Dimensions int
}

// StreamChunk is a single event from a streaming generation.
type StreamChunk struct {
	Delta          string
	ReasoningDelta string
	Done           bool
	FinishReason   string
	Usage          *Usage
	Err            error
	ToolCallID     string
	ToolName       string
	ToolArgsDelta  string
}
