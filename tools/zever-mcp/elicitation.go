package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// DefaultElicitTimeout bounds how long the server waits for the client to
// answer an elicitation request.
const DefaultElicitTimeout = 2 * time.Minute

// Elicitation actions the client may return.
const (
	// ElicitAccept approves the proposed action.
	ElicitAccept = "accept"
	// ElicitDecline rejects the proposed action.
	ElicitDecline = "decline"
	// ElicitCancel aborts the workflow.
	ElicitCancel = "cancel"
)

// elicitResult carries a client's answer to an elicitation request.
type elicitResult struct {
	Action  string         `json:"action"`
	Content map[string]any `json:"content,omitempty"`
}

// senderFunc delivers one server-originated message to the client. It is set
// by run(); a nil sender means elicitation is unavailable (direct handle()
// calls in tests).
type senderFunc func(v any) error

// elicit asks the client to approve a proposed action and awaits the answer.
// It fails closed: decline, cancel, timeout, and context cancellation all
// return an error or non-accept action the caller must honor.
func (s *Server) elicit(ctx context.Context, message string, schema map[string]any) (elicitResult, error) {
	var zero elicitResult

	send := s.getSender()
	if send == nil {
		return zero, errors.New("zever-mcp: elicitation unavailable")
	}

	id := s.nextServerID.Add(1)

	ch := make(chan elicitResult, 1)

	s.mu.Lock()
	if s.pending == nil {
		s.pending = make(map[int64]chan elicitResult)
	}
	s.pending[id] = ch
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.pending, id)
		s.mu.Unlock()
	}()

	rawID, err := json.Marshal(id)
	if err != nil {
		return zero, fmt.Errorf("zever-mcp: marshal elicitation id: %w", err)
	}

	params := map[string]any{"message": message}
	if schema != nil {
		params["requestedSchema"] = schema
	}

	if err := send(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(rawID),
		"method":  "elicitation/create",
		"params":  params,
	}); err != nil {
		return zero, fmt.Errorf("zever-mcp: send elicitation: %w", err)
	}

	timer := time.NewTimer(DefaultElicitTimeout)
	defer timer.Stop()

	select {
	case res := <-ch:
		switch res.Action {
		case ElicitAccept, ElicitDecline, ElicitCancel:
			return res, nil
		default:
			return zero, fmt.Errorf("zever-mcp: unknown elicitation action %q", res.Action)
		}
	case <-ctx.Done():
		return zero, ctx.Err()
	case <-timer.C:
		return zero, errors.New("zever-mcp: elicitation timed out")
	}
}

// deliverResponse routes an incoming client message to a pending elicitation
// when its ID matches. It reports whether the message was consumed.
func (s *Server) deliverResponse(raw json.RawMessage) bool {
	var probe struct {
		ID     *int64          `json:"id"`
		Method string          `json:"method,omitempty"`
		Result json.RawMessage `json:"result,omitempty"`
		Error  *rpcError       `json:"error,omitempty"`
	}

	if err := json.Unmarshal(raw, &probe); err != nil {
		return false
	}

	if probe.ID == nil || probe.Method != "" {
		return false
	}

	if probe.Result == nil && probe.Error == nil {
		return false
	}

	s.mu.Lock()
	ch, ok := s.pending[*probe.ID]
	s.mu.Unlock()

	if !ok {
		return false
	}

	// Malformed answers fail closed as cancellation.
	var res elicitResult
	if probe.Error == nil {
		if err := json.Unmarshal(probe.Result, &res); err != nil {
			res = elicitResult{}
		}
	}
	if res.Action == "" {
		res.Action = ElicitCancel
	}

	select {
	case ch <- res:
	default:
	}

	return true
}
