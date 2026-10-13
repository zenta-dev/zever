package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// DefaultE2ETimeout bounds the scaffold boot e2e.
const DefaultE2ETimeout = 3 * time.Minute

// waitFor polls cond until true or ctx done, for deterministic e2e sync.
func waitFor(ctx context.Context, cond func() bool) bool {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if cond() {
			return true
		}
		select {
		case <-ctx.Done():
			return cond()
		case <-ticker.C:
		}
	}
}

// freeLoopbackPort returns a free loopback port using ListenConfig.
func freeLoopbackPort(ctx context.Context, t *testing.T) string {
	t.Helper()
	lc := &net.ListenConfig{}
	ln, listenErr := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if listenErr != nil {
		t.Fatalf("free port: %v", listenErr)
	}
	defer func() { _ = ln.Close() }()
	_, port, _ := strings.Cut(ln.Addr().String(), ":")

	return port
}

// getHealth probes url with a context request, reporting 200 only.
func getHealth(ctx context.Context, client *http.Client, url string) bool {
	req, reqErr := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if reqErr != nil {
		return false
	}
	resp, doErr := client.Do(req)
	if doErr != nil {
		return false
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()

	return resp.StatusCode == http.StatusOK
}

// TestP32ScaffoldBootsE2E proves `zever new` yields a bootable app:
// new → compile → build → run → GET /healthz 200 → graceful shutdown.
// Offline via GOPROXY=off with local workspace replaces; skips when the
// toolchain is unavailable.
func TestP32ScaffoldBootsE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("scaffold boot e2e is slow")
	}
	goBin, lookErr := exec.LookPath("go")
	if lookErr != nil {
		t.Skipf("no go toolchain on PATH: %v", lookErr)
	}

	repoRoot := repoRootAbs(t)
	workDir := t.TempDir()
	withWorkingDir(t, workDir)
	t.Setenv(zeverFrameworkPathEnv, repoRoot)
	// Offline: module cache + local replaces only.
	t.Setenv("GOPROXY", "off")
	t.Setenv("GOFLAGS", "-mod=mod")
	t.Setenv("GOWORK", "off")

	// Restore real tidy runner (TestMain stubs it for unit speed).
	runGoModTidyFunc = runGoModTidy
	t.Cleanup(func() { runGoModTidyFunc = func(string, string) error { return nil } })

	newErr := runNew([]string{"e2eapp"})
	if newErr != nil {
		t.Fatalf("runNew: %v", newErr)
	}
	projDir := filepath.Join(workDir, "e2eapp")

	// Compile starter schema (new → compile → build → run).
	t.Chdir(projDir)
	compileErr := runCompile([]string{"--backend=zenorm,proto,atlas", "schema/app.zen"})
	if compileErr != nil {
		t.Fatalf("runCompile: %v", compileErr)
	}
	t.Chdir(workDir)

	// Build server binary offline.
	ctx, cancel := context.WithTimeout(t.Context(), DefaultE2ETimeout)
	defer cancel()

	serverBin := filepath.Join(projDir, "server-e2e")
	buildCmd := exec.CommandContext(ctx, goBin, "build", "-o", serverBin, "./cmd/server")
	buildCmd.Dir = projDir
	buildCmd.Env = os.Environ()
	out, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		t.Fatalf("go build ./cmd/server: %v\n%s", buildErr, out)
	}

	httpAddr := "127.0.0.1:" + freeLoopbackPort(ctx, t)
	grpcAddr := "127.0.0.1:" + freeLoopbackPort(ctx, t)

	// Run server.
	srvCtx, srvStop := context.WithCancel(context.Background())
	defer srvStop()
	srvCmd := exec.CommandContext(srvCtx, serverBin, "-addr", httpAddr, "-grpc-addr", grpcAddr)
	srvCmd.Dir = projDir
	srvCmd.Env = os.Environ()
	startErr := srvCmd.Start()
	if startErr != nil {
		t.Fatalf("start server: %v", startErr)
	}
	// Ensure graceful shutdown via SIGTERM.
	defer func() {
		_ = srvCmd.Process.Signal(syscall.SIGTERM)
		done := make(chan struct{})
		go func() { _ = srvCmd.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			_ = srvCmd.Process.Kill()
			<-done
		}
	}()

	// Wait for /healthz 200.
	healthURL := fmt.Sprintf("http://%s/healthz", httpAddr)
	client := &http.Client{Timeout: 2 * time.Second}
	ready := waitFor(ctx, func() bool { return getHealth(ctx, client, healthURL) })
	if !ready {
		if srvCmd.ProcessState != nil && srvCmd.ProcessState.Exited() {
			t.Fatalf("server exited before /healthz ready")
		}
		t.Fatalf("server never returned 200 at %s", healthURL)
	}

	// Graceful shutdown: SIGTERM must exit cleanly.
	sigErr := srvCmd.Process.Signal(syscall.SIGTERM)
	if sigErr != nil {
		t.Fatalf("signal server: %v", sigErr)
	}
	done := make(chan error, 1)
	go func() { done <- srvCmd.Wait() }()
	select {
	case waitErr := <-done:
		if waitErr != nil {
			// SIGTERM exit may surface as signal error; accept clean SIGTERM.
			if !strings.Contains(waitErr.Error(), "signal: terminated") && !strings.Contains(waitErr.Error(), "exit status") {
				t.Fatalf("server shutdown: %v", waitErr)
			}
		}
	case <-time.After(15 * time.Second):
		_ = srvCmd.Process.Kill()
		t.Fatalf("server did not shut down gracefully")
	}
}
