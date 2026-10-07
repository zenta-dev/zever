package mcpclient

import (
	"errors"
	"fmt"
)

var (
	// ErrProtocol is returned when the wire conversation violates JSON-RPC
	// shape (bad JSON, mismatched IDs, missing results).
	ErrProtocol = errors.New("mcpclient: protocol error")
	// ErrRPC is returned when the server answers a call with a JSON-RPC
	// error object.
	ErrRPC = errors.New("mcpclient: rpc error")
)

// RPCError reports a server-side JSON-RPC error object.
type RPCError struct {
	// Code is the JSON-RPC error code.
	Code int
	// Message is the server-provided message.
	Message string
}

// Error returns a human-readable RPC error message.
func (e RPCError) Error() string {
	return fmt.Sprintf("mcpclient: rpc error %d: %s", e.Code, e.Message)
}

// Unwrap returns ErrRPC.
func (e RPCError) Unwrap() error { return ErrRPC }
