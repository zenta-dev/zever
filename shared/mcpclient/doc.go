// Package mcpclient speaks the Model Context Protocol as a client over a
// newline-delimited JSON-RPC 2.0 transport.
//
// It owns the initialize handshake, tool listing, and tool calls against an
// MCP server reachable through any io.Reader/io.Writer pair (a spawned
// server process, an in-memory pipe in tests). It does not spawn processes
// itself; see DialStdio for that.
package mcpclient
