package main

import (
	"context"
	"io"
	"log"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
)

// stdioRW adapts a reader/writer pair to the ReadWriteCloser that
// jsonrpc2.NewStream expects. Close is a no-op: stdin/stdout are owned by
// the process, not the connection.
type stdioRW struct {
	stdin  io.Reader
	stdout io.Writer
}

// Read reads a chunk of the framed input stream.
func (s stdioRW) Read(p []byte) (int, error) {
	return s.stdin.Read(p)
}

// Write writes a chunk of the framed output stream.
func (s stdioRW) Write(p []byte) (int, error) {
	return s.stdout.Write(p)
}

// Close releases nothing: the process owns stdin/stdout.
func (s stdioRW) Close() error {
	return nil
}

// run wires stdio to the LSP server and blocks until the peer disconnects.
// It returns a process exit code: 0 on a clean shutdown, 1 when the
// connection terminated with an error. All logging goes to stderr so the
// stdio JSON-RPC stream stays clean.
func run(ctx context.Context, stdin io.Reader, stdout io.Writer) int {
	srv := NewServer()

	stream := jsonrpc2.NewStream(stdioRW{stdin: stdin, stdout: stdout})

	_, conn, client := protocol.NewServer(ctx, srv, stream)
	srv.setClient(client)

	<-conn.Done()

	if err := conn.Err(); err != nil {
		log.Printf("zever-lsp: server exited: %v", err)

		return 1
	}

	return 0
}
