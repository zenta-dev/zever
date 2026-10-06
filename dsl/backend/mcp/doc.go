// Package mcp implements the "mcp" backend.Backend: it renders a resolved
// *ir.Schema into MCP tool manifests, one per module plus one merged manifest
// covering every module, so each service RPC becomes a model-callable tool
// with JSON Schema input and output.
package mcp
