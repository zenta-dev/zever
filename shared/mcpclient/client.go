package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

// request is a JSON-RPC 2.0 request.
type request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// response is a JSON-RPC 2.0 response.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcErrorWire   `json:"error,omitempty"`
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
type Client struct {
	enc  *json.Encoder
	dec  *json.Decoder
	mu   sync.Mutex
	next atomic.Int64
}

// New builds a Client over r and w. It performs no I/O until the first call.
func New(r io.Reader, w io.Writer) *Client {
	return &Client{enc: json.NewEncoder(w), dec: json.NewDecoder(r)}
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

		var resp response
		if err := c.dec.Decode(&resp); err != nil {
			return fmt.Errorf("mcpclient: decode %s: %w", method, err)
		}

		if resp.ID == nil {
			continue
		}

		if *resp.ID != id {
			return fmt.Errorf("mcpclient: response id mismatch: %w", ErrProtocol)
		}

		if resp.Error != nil {
			return RPCError{Code: resp.Error.Code, Message: resp.Error.Message}
		}

		if out == nil {
			return nil
		}

		if len(resp.Result) == 0 {
			return fmt.Errorf("mcpclient: empty %s result: %w", method, ErrProtocol)
		}

		if err := json.Unmarshal(resp.Result, out); err != nil {
			return fmt.Errorf("mcpclient: decode %s result: %w", method, err)
		}

		return nil
	}
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
