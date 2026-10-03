// Package ollama provides an AI implementation backed by an Ollama server.
//
// Non-loopback endpoints must use https and TLS certificates are verified by
// default. Plain http (and, for test servers, TLS verification) requires the
// explicit AllowInsecure opt-in; loopback http is always permitted for local
// development against a default Ollama server.
package ollama
