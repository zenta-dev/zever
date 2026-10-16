package container

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	rpprof "runtime/pprof"
	rtrace "runtime/trace"
	"strconv"
	"strings"
	"time"

	"github.com/zenta-dev/zever/core/log"
)

// pprofReadHeaderTimeout bounds how long the pprof listener waits for request
// headers. There is deliberately no write timeout: /debug/pprof/profile and
// /trace stream for as long as the caller asks (?seconds=N).
const pprofReadHeaderTimeout = 5 * time.Second

// PprofServer is a running net/http/pprof listener started by StartPprof. It
// serves on its own loopback-only address, never on the application's public
// listeners. A nil *PprofServer is valid and means profiling is off; every
// method is nil-safe and safe for concurrent use.
type PprofServer struct {
	srv  *http.Server
	addr net.Addr
	done chan struct{}
}

// StartPprof serves the standard net/http/pprof endpoints under
// /debug/pprof/ on addr, a listener separate from the application's own. An
// empty addr returns (nil, nil): profiling is off by default.
//
// addr must bind loopback only (e.g. "127.0.0.1:6060" or "localhost:6060"):
// pprof exposes heap contents and goroutine stacks, so a wildcard or public
// host is rejected. Reach it from outside the host with an SSH tunnel or
// `kubectl port-forward`. The listener is opened before StartPprof returns, so
// a bad or busy address fails the boot instead of being swallowed. The server
// stops when Shutdown is called; logger may be nil.
func StartPprof(addr string, logger log.Logger) (*PprofServer, error) {
	if addr == "" {
		return nil, nil //nolint:nilnil // nil means "pprof off"; nil-safe methods keep call sites branch-free.
	}

	if !isLoopbackListenAddr(addr) {
		return nil, fmt.Errorf("container: pprof addr %q must bind loopback only (use 127.0.0.1:PORT or localhost:PORT, then SSH tunnel or kubectl port-forward)", addr)
	}

	lis, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("container: pprof listen on %q: %w", addr, err)
	}

	p := &PprofServer{
		srv:  &http.Server{Handler: PprofHandler(), ReadHeaderTimeout: pprofReadHeaderTimeout},
		addr: lis.Addr(),
		done: make(chan struct{}),
	}

	if logger != nil {
		logger.Warn().Str("addr", lis.Addr().String()).Msg("pprof enabled: serving /debug/pprof on a loopback-only listener; do not expose it publicly")
	}

	go func() {
		defer close(p.done)

		if serveErr := p.srv.Serve(lis); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) && logger != nil {
			logger.Error().Err(serveErr).Msg("pprof server stopped")
		}
	}()

	return p, nil
}

// PprofHandler returns a handler serving the runtime profiles under
// /debug/pprof/: an index, any named runtime profile (heap, goroutine,
// allocs, block, mutex, threadcreate; ?debug=N as in net/http/pprof), plus
// /profile (CPU, ?seconds=N) and /trace (execution trace, ?seconds=N). /cmdline is
// deliberately not served: argv often carries secrets.
//
// It is built on runtime/pprof and runtime/trace rather than net/http/pprof
// on purpose: importing net/http/pprof registers its handlers on
// http.DefaultServeMux from init, which would silently expose profiling on
// any program that links this package and serves the default mux.
func PprofHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprofIndex)
	mux.HandleFunc("/debug/pprof/profile", pprofCPU)
	mux.HandleFunc("/debug/pprof/trace", pprofTrace)

	return mux
}

const (
	pprofDefaultSeconds = 30
	pprofMaxSeconds     = 300
)

func pprofIndex(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/debug/pprof/")
	if name == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")

		for _, p := range rpprof.Profiles() {
			_, _ = fmt.Fprintf(w, "%s (%d)\n", p.Name(), p.Count())
		}

		_, _ = fmt.Fprint(w, "profile\ntrace\n")

		return
	}

	p := rpprof.Lookup(name)
	if p == nil {
		http.Error(w, "unknown profile: "+name, http.StatusNotFound)

		return
	}

	debug, _ := strconv.Atoi(r.URL.Query().Get("debug"))

	w.Header().Set("X-Content-Type-Options", "nosniff")

	if debug > 0 {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	}

	_ = p.WriteTo(w, debug)
}

// pprofSeconds parses ?seconds=N, defaulting to 30 and capped at 300.
func pprofSeconds(r *http.Request) (time.Duration, bool) {
	raw := r.URL.Query().Get("seconds")
	if raw == "" {
		return pprofDefaultSeconds * time.Second, true
	}

	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > pprofMaxSeconds {
		return 0, false
	}

	return time.Duration(n) * time.Second, true
}

// pprofSleep waits d or until the request is cancelled.
func pprofSleep(r *http.Request, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
	case <-r.Context().Done():
	}
}

func pprofCPU(w http.ResponseWriter, r *http.Request) {
	d, ok := pprofSeconds(r)
	if !ok {
		http.Error(w, "seconds must be an integer in [1, 300]", http.StatusBadRequest)

		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="profile"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := rpprof.StartCPUProfile(w); err != nil {
		http.Error(w, "cpu profiling already running: "+err.Error(), http.StatusInternalServerError)

		return
	}

	pprofSleep(r, d)
	rpprof.StopCPUProfile()
}

func pprofTrace(w http.ResponseWriter, r *http.Request) {
	d, ok := pprofSeconds(r)
	if !ok {
		http.Error(w, "seconds must be an integer in [1, 300]", http.StatusBadRequest)

		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="trace"`)
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if err := rtrace.Start(w); err != nil {
		http.Error(w, "tracing already running: "+err.Error(), http.StatusInternalServerError)

		return
	}

	pprofSleep(r, d)
	rtrace.Stop()
}

// Addr returns the bound listener address, or nil when profiling is off.
func (p *PprofServer) Addr() net.Addr {
	if p == nil {
		return nil
	}

	return p.addr
}

// Shutdown stops the listener, waiting for in-flight requests up to ctx, and
// then for the serve goroutine to exit. It is a no-op on a nil server.
func (p *PprofServer) Shutdown(ctx context.Context) error {
	if p == nil {
		return nil
	}

	err := p.srv.Shutdown(ctx)

	select {
	case <-p.done:
	case <-ctx.Done():
	}

	return err
}

// isLoopbackListenAddr reports whether addr (host:port) names a loopback
// host. Empty and wildcard hosts bind every interface and an unparsable
// address fails closed.
func isLoopbackListenAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}

	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
