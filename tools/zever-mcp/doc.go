// Package main implements zever-mcp, a Model Context Protocol server that
// exposes the zever schema compiler to AI agents over stdio.
//
// It speaks newline-delimited JSON-RPC 2.0 (the MCP stdio transport) and
// implements the initialize, ping, tools/list and tools/call methods. Tools
// compile .zen schemas and summarize or explain the resolved IR, so an agent
// can introspect and validate a zever project without reading compiler source.
package main
