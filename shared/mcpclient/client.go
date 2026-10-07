package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

// request is a JSON-RPC 2.0 request.
type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// rpcErrorWire is the wire shape of a JSON-RPC error object.
type rpcErrorWire struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Tool describes one server tool.
type Tool struct {
	// Name is the unique tool identifier.
	Name string
	// Description is the human-readable purpose.
	Description string
	// InputSchema is the JSON Schema object for arguments.
	InputSchema map[string]any
}

// TextContent is one text content block in a tool result.
type TextContent struct {
	// Type is always "text".
	Type string
	// Text is the block payload.
	Text string
}

// CallResult is a tools/call result.
type CallResult struct {
	// Content carries the result blocks.
	Content []TextContent
	// IsError marks a tool execution failure.
	IsError bool
}

// Client speaks MCP to one server over a JSON-RPC transport. Responses are
// matched by request ID; server notifications (messages without IDs) are
// skipped. It is safe for concurrent use. Calls serialize on one mutex, so
// a server that stops responding blocks later calls until the transport
// read fails; callers should bound this with timeouts or cancel between
// calls (cancellation is honored before each read).
//
// Incoming server requests (sampling/create, elicitation/create) dispatch
// to the configured hooks while a call waits. Hooks must not call Client
// methods: the transport lock is held during dispatch.
type Client struct {
	enc  *json.Encoder
	dec  *json.Decoder
	mu   sync.Mutex
	next atomic.Int64

	onSampling Sampler
	onElicit   ElicitPolicy
}

// New builds a Client over r and w. It performs no I/O until the first call.
func New(r io.Reader, w io.Writer, opts ...Option) *Client {
	c := &Client{enc: json.NewEncoder(w), dec: json.NewDecoder(r)}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// DialStdio spawns path as an MCP stdio server and returns a Client over its
// pipes plus the child command. The caller owns cmd: wait for it, and kill
// it on context cancellation.
func DialStdio(ctx context.Context, path string, args ...string) (*Client, *exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, path, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("mcpclient: stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()

		return nil, nil, fmt.Errorf("mcpclient: stdout pipe: %w", err)
	}

	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()

		return nil, nil, fmt.Errorf("mcpclient: start %q: %w", path, err)
	}

	return New(stdout, stdin), cmd, nil
}

// Dial retry bounds for DialStdioRetry.
const (
	// DefaultDialAttempts caps spawn attempts.
	DefaultDialAttempts = 3
	// DefaultDialInitialBackoff is the first retry delay, doubling each time.
	DefaultDialInitialBackoff = 200 * time.Millisecond
	// DefaultDialMaxBackoff caps the retry delay.
	DefaultDialMaxBackoff = 5 * time.Second
)

// DialOptions configures DialStdioRetry.
type DialOptions struct {
	// Attempts caps spawn attempts; <= 0 selects DefaultDialAttempts.
	Attempts int
	// InitialBackoff is the first retry delay; <= 0 selects the default.
	InitialBackoff time.Duration
	// MaxBackoff caps the retry delay; <= 0 selects the default.
	MaxBackoff time.Duration
}

// DialStdioRetry spawns path like DialStdio, retrying spawn failures with
// exponential backoff. It returns the first success or a wrapped final error.
func DialStdioRetry(ctx context.Context, path string, opts DialOptions, args ...string) (*Client, *exec.Cmd, error) {
	attempts := opts.Attempts
	if attempts <= 0 {
		attempts = DefaultDialAttempts
	}

	backoff := opts.InitialBackoff
	if backoff <= 0 {
		backoff = DefaultDialInitialBackoff
	}

	maxBackoff := opts.MaxBackoff
	if maxBackoff <= 0 {
		maxBackoff = DefaultDialMaxBackoff
	}

	var err error

	for attempt := 1; ; attempt++ {
		var c *Client
		var cmd *exec.Cmd

		c, cmd, err = DialStdio(ctx, path, args...)
		if err == nil {
			return c, cmd, nil
		}

		if attempt >= attempts {
			break
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()

			return nil, nil, fmt.Errorf("mcpclient: dial %q: %w", path, ctx.Err())
		case <-timer.C:
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}

	return nil, nil, fmt.Errorf("mcpclient: dial %q failed after %d attempts: %w", path, attempts, err)
}

// call sends one request and decodes the matching response.
func (c *Client) call(ctx context.Context, method string, params, out any) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	id := c.next.Add(1)

	if err := c.enc.Encode(request{JSONRPC: "2.0", ID: id, Method: method, Params: params}); err != nil {
		return fmt.Errorf("mcpclient: encode %s: %w", method, err)
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var msg inbound
		if err := c.dec.Decode(&msg); err != nil {
			return fmt.Errorf("mcpclient: decode %s: %w", method, err)
		}

		if msg.Method != "" && msg.Result == nil && msg.Error == nil {
			if err := c.dispatchRequest(ctx, msg.ID, msg.Method, msg.Params); err != nil {
				return err
			}

			continue
		}

		if msg.ID == nil {
			continue
		}

		if *msg.ID != id {
			return fmt.Errorf("mcpclient: response id mismatch: %w", ErrProtocol)
		}

		if msg.Error != nil {
			return RPCError{Code: msg.Error.Code, Message: msg.Error.Message}
		}

		if out == nil {
			return nil
		}

		if len(msg.Result) == 0 {
			return fmt.Errorf("mcpclient: empty %s result: %w", method, ErrProtocol)
		}

		if err := json.Unmarshal(msg.Result, out); err != nil {
			return fmt.Errorf("mcpclient: decode %s result: %w", method, err)
		}

		return nil
	}
}

// inbound is the wire shape of any incoming message: responses carry
// result/error, server requests carry a method.
type inbound struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcErrorWire   `json:"error,omitempty"`
}

// dispatchRequest answers one server-initiated request. Notifications (nil
// ID) return before the switch: hooks never run for them.
func (c *Client) dispatchRequest(ctx context.Context, id *int64, method string, params json.RawMessage) error {
	if id == nil {
		return nil
	}

	answer := func(result any, rpcErr *RPCError) error {
		if rpcErr != nil {
			return c.sendResponse(id, nil, rpcErr)
		}

		raw, err := json.Marshal(result)
		if err != nil {
			return c.sendResponse(id, nil, &RPCError{Code: -32603, Message: err.Error()})
		}

		return c.sendResponse(id, raw, nil)
	}

	switch method {
	case "sampling/create":
		if c.onSampling == nil {
			return answer(nil, &RPCError{Code: -32601, Message: "sampling not supported"})
		}

		var p struct {
			Messages []map[string]any `json:"messages"`
		}
		rest := map[string]any{}

		if len(params) > 0 {
			var full map[string]any
			if err := json.Unmarshal(params, &full); err != nil {
				return answer(nil, &RPCError{Code: -32602, Message: "invalid sampling params"})
			}

			if err := json.Unmarshal(params, &p); err != nil {
				return answer(nil, &RPCError{Code: -32602, Message: "invalid sampling params"})
			}

			rest = full
		}

		res, err := c.onSampling(ctx, SampleRequest{Messages: p.Messages, Params: rest})
		if err != nil {
			return answer(nil, &RPCError{Code: -32603, Message: err.Error()})
		}

		return answer(res.Content, nil)
	case "elicitation/create":
		var p struct {
			Message string         `json:"message"`
			Schema  map[string]any `json:"requestedSchema"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &p); err != nil {
				return answer(nil, &RPCError{Code: -32602, Message: "invalid elicitation params"})
			}
		}

		action := ElicitDecline
		var content map[string]any

		if c.onElicit != nil {
			var err error
			var acted ElicitAction
			acted, content, err = c.onElicit(ctx, ElicitRequest{Message: p.Message, Schema: p.Schema})
			if err != nil {
				return answer(nil, &RPCError{Code: -32603, Message: err.Error()})
			}

			action = acted
		}

		return answer(map[string]any{"action": string(action), "content": content}, nil)
	default:
		return answer(nil, &RPCError{Code: -32601, Message: "method not found: " + method})
	}
}

// sendResponse encodes one response or error for id.
func (c *Client) sendResponse(id *int64, result json.RawMessage, rpcErr *RPCError) error {
	msg := map[string]any{"jsonrpc": "2.0", "id": *id}

	if rpcErr != nil {
		msg["error"] = map[string]any{"code": rpcErr.Code, "message": rpcErr.Message}
	} else {
		msg["result"] = result
	}

	if err := c.enc.Encode(msg); err != nil {
		return fmt.Errorf("mcpclient: encode response: %w", err)
	}

	return nil
}

// Initialize performs the MCP handshake and returns the negotiated protocol
// version.
func (c *Client) Initialize(ctx context.Context) (string, error) {
	var out struct {
		ProtocolVersion string `json:"protocolVersion"`
	}

	if err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "zever-mcpclient", "version": "0.0.0"},
	}, &out); err != nil {
		return "", err
	}

	if out.ProtocolVersion == "" {
		return "", fmt.Errorf("mcpclient: empty protocol version: %w", ErrProtocol)
	}

	return out.ProtocolVersion, nil
}

// ListTools returns the server's tool catalog.
func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	var out struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	}

	if err := c.call(ctx, "tools/list", nil, &out); err != nil {
		return nil, err
	}

	tools := make([]Tool, 0, len(out.Tools))
	for _, t := range out.Tools {
		tools = append(tools, Tool{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		})
	}

	return tools, nil
}

// CallTool invokes a server tool with arguments.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (CallResult, error) {
	var out struct {
		Content []TextContent `json:"content"`
		IsError bool          `json:"isError"`
	}

	if err := c.call(ctx, "tools/call", map[string]any{"name": name, "arguments": args}, &out); err != nil {
		return CallResult{}, err
	}

	return CallResult{Content: out.Content, IsError: out.IsError}, nil
}
