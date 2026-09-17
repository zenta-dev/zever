package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

// pipeEnds splits two io.Pipes into a server-side (stdin, stdout) pair and
// a client-side stream, so a test can speak LSP to run() or a subprocess
// without touching the real stdio or the network.
type pipeEnds struct {
	serverStdin  io.Reader
	serverStdout io.Writer

	clientStream jsonrpc2.Stream
}

// newPipeEnds returns connected ends: the server reads what the client
// writes and vice versa. Closing the client stream closes the server's
// stdin, which the server observes as a clean EOF.
func newPipeEnds() *pipeEnds {
	clientToServerR, clientToServerW := io.Pipe()
	serverToClientR, serverToClientW := io.Pipe()

	return &pipeEnds{
		serverStdin:  clientToServerR,
		serverStdout: serverToClientW,
		clientStream: jsonrpc2.NewStream(pipeStream{reader: serverToClientR, writer: clientToServerW}),
	}
}

// pipeStream adapts a pipe reader/writer pair to the ReadWriteCloser
// jsonrpc2 streams need. Close shuts both directions: closing the writer
// delivers a clean EOF to the peer's read loop, closing the reader unblocks
// the local one.
type pipeStream struct {
	reader *io.PipeReader
	writer *io.PipeWriter
}

func (p pipeStream) Read(b []byte) (int, error)  { return p.reader.Read(b) }
func (p pipeStream) Write(b []byte) (int, error) { return p.writer.Write(b) }
func (p pipeStream) Close() error {
	rerr := p.reader.Close()

	return errors.Join(rerr, p.writer.Close())
}

// waitFor polls cond until it holds or the timeout elapses.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

// driveHandshake runs initialize/initialized/didOpen against srv and waits
// for the first publishDiagnostics round covering docURI.
func driveHandshake(ctx context.Context, t *testing.T, srv protocol.Server, fake *fakeClient, docURI, content string) {
	t.Helper()

	res, err := srv.Initialize(ctx, &protocol.InitializeParams{})
	if err != nil {
		t.Fatalf("Initialize over pipe: %v", err)
	}

	if res.ServerInfo.Name != serverName {
		t.Fatalf("ServerInfo.Name = %q, want %q", res.ServerInfo.Name, serverName)
	}

	version, ok := res.ServerInfo.Version.Get()
	if !ok || version != serverVersion {
		t.Fatalf("ServerInfo.Version = (%q, %v), want (%q, true)", version, ok, serverVersion)
	}

	if err := srv.Initialized(ctx, &protocol.InitializedParams{}); err != nil {
		t.Fatalf("Initialized over pipe: %v", err)
	}

	if err := srv.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{URI: uriOf(docURI), Text: content},
	}); err != nil {
		t.Fatalf("DidOpen over pipe: %v", err)
	}

	path, ok := uriToPath(docURI)
	if !ok {
		t.Fatalf("uriToPath(%s) failed", docURI)
	}

	wantURI := string(pathToURI(path))

	waitFor(t, 5*time.Second, "publishDiagnostics", func() bool {
		_, ok := fake.published(wantURI)

		return ok
	})
}

// uriOf converts a test URI string to the protocol's URI type.
func uriOf(s string) uri.URI {
	return uri.URI(s)
}

// shutdownExchange sends shutdown/exit, then closes the client connection.
// Closing the stream shuts the server's stdin, which the server observes as
// a clean EOF and returns from run with code 0.
func shutdownExchange(ctx context.Context, t *testing.T, srv protocol.Server, conn jsonrpc2.Conn) {
	t.Helper()

	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown over pipe: %v", err)
	}

	if err := srv.Exit(ctx); err != nil {
		t.Fatalf("Exit over pipe: %v", err)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("client conn.Close: %v", err)
	}
}

func TestRun_stdioLifecycle(t *testing.T) {
	const docURI = "file:///tmp/zever-lsp-run-test.zen"

	ends := newPipeEnds()
	fake := &fakeClient{}

	runCh := make(chan int, 1)

	go func() {
		runCh <- run(context.Background(), ends.serverStdin, ends.serverStdout)
	}()

	ctx := context.Background()
	ctx, conn, srv := protocol.NewClient(ctx, fake, ends.clientStream)

	driveHandshake(ctx, t, srv, fake, docURI, serverBrokenDoc)
	shutdownExchange(ctx, t, srv, conn)

	select {
	case code := <-runCh:
		if code != 0 {
			t.Errorf("run() = %d, want 0 on clean shutdown", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run() did not return after shutdown/exit/EOF")
	}
}

// errReader fails every read, simulating a broken stdio transport.
type errReader struct {
	err error
}

func (e errReader) Read([]byte) (int, error) {
	return 0, e.err
}

func TestRun_transportError(t *testing.T) {
	if code := run(context.Background(), errReader{err: errors.New("boom")}, io.Discard); code != 1 {
		t.Errorf("run() with failing stdin = %d, want 1", code)
	}
}

func TestMain_subprocessCover(t *testing.T) {
	const docURI = "file:///tmp/zever-lsp-main-test.zen"

	tmp := t.TempDir()
	bin := filepath.Join(tmp, "zever-lsp")
	coverDir := filepath.Join(tmp, "coverdata")

	if err := os.MkdirAll(coverDir, 0o755); err != nil {
		t.Fatalf("mkdir coverdata: %v", err)
	}

	build := exec.CommandContext(t.Context(), "go", "build", "-cover", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build -cover: %v\n%s", err, out)
	}

	cmd := exec.CommandContext(t.Context(), bin)
	cmd.Env = append(os.Environ(), "GOCOVERDIR="+coverDir)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("StdinPipe: %v", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("StdoutPipe: %v", err)
	}

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// Tee the raw wire bytes so the test can assert on framing and method
	// names, not just decoded params.
	var wire bytes.Buffer

	var wireMu sync.Mutex

	tee := io.TeeReader(stdout, &lockedWriter{mu: &wireMu, w: &wire})

	if startErr := cmd.Start(); startErr != nil {
		t.Fatalf("start subprocess: %v", startErr)
	}

	fake := &fakeClient{}
	ctx := context.Background()
	ctx, conn, srv := protocol.NewClient(ctx, fake, jsonrpc2.NewStream(rwAdapter{r: tee, w: stdin}))

	driveHandshake(ctx, t, srv, fake, docURI, serverBrokenDoc)
	shutdownExchange(ctx, t, srv, conn)

	if waitErr := cmd.Wait(); waitErr != nil {
		t.Fatalf("subprocess exit: %v\nstderr:\n%s", waitErr, stderr.String())
	}

	if code := cmd.ProcessState.ExitCode(); code != 0 {
		t.Errorf("subprocess exit code = %d, want 0\nstderr:\n%s", code, stderr.String())
	}

	wireMu.Lock()
	raw := wire.String()
	wireMu.Unlock()

	for _, want := range []string{"Content-Length:", "publishDiagnostics", "zever-lsp", "capabilities"} {
		if !bytes.Contains([]byte(raw), []byte(want)) {
			t.Errorf("wire bytes lack %q", want)
		}
	}

	entries, err := os.ReadDir(coverDir)
	if err != nil {
		t.Fatalf("read coverdata: %v", err)
	}

	if len(entries) == 0 {
		t.Errorf("GOCOVERDIR %s is empty, want coverage output from main()", coverDir)
	}

	// Prove main() itself ran under coverage: convert the child's coverdata
	// and require main.go's main to be fully covered.
	funcCov := exec.CommandContext(t.Context(), "go", "tool", "covdata", "func", "-i="+coverDir) //nolint:gosec // test-only: coverDir is a t.TempDir() path, never user input

	out, covErr := funcCov.CombinedOutput()
	if covErr != nil {
		t.Fatalf("go tool covdata func: %v\n%s", covErr, out)
	}

	mainCovered := false

	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "main.go:") && strings.Contains(line, "100.0%") {
			mainCovered = true
		}
	}

	if !mainCovered {
		t.Errorf("subprocess coverdata lacks 100%% main coverage:\n%s", out)
	}
}

// rwAdapter joins a reader and a write-closer into the ReadWriteCloser a
// client-side jsonrpc2 stream needs. Close shuts the subprocess's stdin,
// which the server observes as a clean EOF.
type rwAdapter struct {
	r io.Reader
	w io.WriteCloser
}

func (a rwAdapter) Read(b []byte) (int, error)  { return a.r.Read(b) }
func (a rwAdapter) Write(b []byte) (int, error) { return a.w.Write(b) }
func (a rwAdapter) Close() error                { return a.w.Close() }

// lockedWriter guards a bytes.Buffer shared between the conn read loop and
// the test goroutine.
type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.w.Write(b)
}
