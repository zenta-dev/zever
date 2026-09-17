package main

import "encoding/json"

// This file defines the `zever tinker` wire protocol: the line-delimited JSON
// conversation between the root-module `zever tinker` process (the client,
// which owns the yaegi REPL) and a per-project tinker shim subprocess (the
// server, scaffolded by `zever generate tinker` and compiled inside the
// target project's own Go module).
//
// # Why a subprocess at all
//
// A live Rails-style console needs the project's real container: its real DB
// handle, its real cache, its real queue. The `zever` binary cannot import a
// target project's `internal/app` package — it is a different Go module, and
// Go forbids cross-module imports of `internal/...` even if `zever` somehow
// knew which project it was pointed at ahead of time. So the container is
// built in a small shim compiled inside the project, and `zever tinker` talks
// to it over pipes.
//
// # Why a fixed verb set
//
// The shim exposes a narrow, closed set of verbs rather than arbitrary Go
// method/type access. Arbitrary access would mean regenerating yaegi symbol
// tables for the project's generated ORM code on every schema change; that
// was considered and rejected. The verb set below is deliberately small and
// stable.
//
// # Framing
//
// Requests: one tinkerRequest as compact JSON per line on the shim's stdin.
//
// Responses: one tinkerResponse as compact JSON per line on the shim's
// stdout, each prefixed with tinkerFramePrefix. The prefix exists because the
// shim builds a real container, and a real container may hold a logger that
// also writes to stdout. Any stdout line without the prefix is not protocol
// traffic — the client forwards it verbatim to its own stderr so the
// developer still sees their app's output.
//
// # Duplication
//
// The shim lives in a different module and therefore cannot import this
// package. `zever generate tinker` emits an equivalent set of type
// declarations as literal Go source into the generated shim. Both copies
// carry a "must stay in sync with cmd/zever/tinker_protocol.go" comment so
// future drift is visible. The JSON schema below is the contract of record.

// tinkerFramePrefix marks a stdout line as a protocol response. See "Framing".
const tinkerFramePrefix = "@@zever-tinker@@"

// Verb names. The full closed set of operations a shim understands.
const (
	verbPing        = "ping"
	verbDBQuery     = "db.query"
	verbDBExec      = "db.exec"
	verbCacheGet    = "cache.get"
	verbCacheSet    = "cache.set"
	verbCacheDelete = "cache.delete"
	verbCacheExists = "cache.exists"
	verbQueuePush   = "queue.push"
	verbQueueLength = "queue.length"
	verbJobDispatch = "job.dispatch"
)

// tinkerRequest is one line of JSON sent from `zever tinker` to the shim.
//
//	{"verb":"db.query","args":{"sql":"select 1","args":[]}}
type tinkerRequest struct {
	Verb string          `json:"verb"`
	Args json.RawMessage `json:"args,omitempty"`
}

// tinkerResponse is one framed line of JSON sent back by the shim. Exactly
// one of Result / Error is meaningful: a non-empty Error means the verb
// failed and Result must be ignored.
//
//	@@zever-tinker@@{"result":{"columns":["1"],"rows":[[1]]}}
//	@@zever-tinker@@{"error":"[sqlite] query error: no such table: todos"}
type tinkerResponse struct {
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// dbQueryArgs are the arguments to "db.query".
type dbQueryArgs struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args,omitempty"`
}

// dbQueryResult is the result of "db.query". Rows are positional and align
// with Columns. Values are whatever the driver scanned, JSON-encoded; []byte
// column values are converted to strings by the shim so they survive the
// round trip as text rather than base64.
type dbQueryResult struct {
	Columns []string `json:"columns"`
	Rows    [][]any  `json:"rows"`
}

// dbExecArgs are the arguments to "db.exec". It has no result payload.
type dbExecArgs struct {
	SQL  string `json:"sql"`
	Args []any  `json:"args,omitempty"`
}

// cacheGetArgs are the arguments to "cache.get".
type cacheGetArgs struct {
	Key string `json:"key"`
}

// cacheGetResult is the result of "cache.get". Value is the cached bytes
// interpreted as a UTF-8 string.
type cacheGetResult struct {
	Value string `json:"value"`
}

// cacheSetArgs are the arguments to "cache.set". A non-positive TTLSeconds
// means "no expiry", matching cache.Cache's own convention for a zero ttl.
type cacheSetArgs struct {
	Key        string `json:"key"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// cacheDeleteArgs are the arguments to "cache.delete". No result payload.
type cacheDeleteArgs struct {
	Key string `json:"key"`
}

// cacheExistsArgs are the arguments to "cache.exists".
type cacheExistsArgs struct {
	Key string `json:"key"`
}

// boolResult is the result of the verbs that answer a yes/no question
// ("cache.exists").
type boolResult struct {
	Value bool `json:"value"`
}

// queuePushArgs are the arguments to "queue.push". No result payload.
type queuePushArgs struct {
	Topic   string            `json:"topic"`
	Payload string            `json:"payload"`
	Headers map[string]string `json:"headers,omitempty"`
}

// queueLengthArgs are the arguments to "queue.length".
type queueLengthArgs struct {
	Topic string `json:"topic"`
}

// int64Result is the result of the verbs that answer with a count
// ("queue.length").
type int64Result struct {
	Value int64 `json:"value"`
}

// jobDispatchArgs are the arguments to "job.dispatch". Args is the job's own
// argument payload, passed through to Dispatcher.Dispatch as already-decoded
// JSON. No result payload.
type jobDispatchArgs struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args,omitempty"`
}
