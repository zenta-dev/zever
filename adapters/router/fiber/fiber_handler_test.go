package fiber

import (
	"bytes"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	recovermw "github.com/gofiber/fiber/v2/middleware/recover"
)

// errReader fails every Read, exercising the adaptor's body-read error path.
type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read failed") }

// adaptorEquivalenceApp builds the route table shared by both handlers under
// test so their responses can be compared byte for byte.
func adaptorEquivalenceApp() *fiber.App {
	app := fiber.New(fiber.Config{AppName: "app", CaseSensitive: true})
	app.Use(recovermw.New())
	app.Add(http.MethodGet, "/hello/:name", func(c *fiber.Ctx) error {
		return c.SendString("hello " + c.Params("name"))
	})
	app.Add(http.MethodPost, "/echo", func(c *fiber.Ctx) error {
		return c.Send(c.Body())
	})
	app.Add(http.MethodGet, "/multi", func(c *fiber.Ctx) error {
		c.Set("X-One", "1")
		c.Set("X-Two", "2")

		return c.Status(fiber.StatusCreated).SendString("created")
	})

	return app
}

// TestNewHTTPHandlerMatchesAdaptor pins newHTTPHandler to the observable
// behavior of the upstream gofiber/adaptor.FiberApp it replaces: status,
// headers and body must match across request shapes, body handling and
// RemoteAddr parsing. Requests are rebuilt for each handler because the
// adaptor consumes the body and may rewrite RemoteAddr.
func TestNewHTTPHandlerMatchesAdaptor(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		target     string
		body       func() io.Reader
		remoteAddr string
		headers    map[string]string
	}{
		{name: "get", method: http.MethodGet, target: "/hello/world", remoteAddr: "192.0.2.1:1234"},
		{name: "get_query", method: http.MethodGet, target: "/hello/world?a=1&b=2", remoteAddr: "192.0.2.1:1234"},
		{name: "post_body", method: http.MethodPost, target: "/echo", body: func() io.Reader { return strings.NewReader("payload") }, remoteAddr: "203.0.113.7:80"},
		{name: "empty_body", method: http.MethodPost, target: "/echo", body: func() io.Reader { return bytes.NewReader(nil) }, remoteAddr: "10.0.0.1:9999"},
		{name: "body_read_error", method: http.MethodPost, target: "/echo", body: func() io.Reader { return errReader{} }, remoteAddr: "192.0.2.1:1234"},
		{name: "missing_port", method: http.MethodGet, target: "/hello/world", remoteAddr: "192.0.2.1"},
		{name: "ipv6", method: http.MethodGet, target: "/hello/world", remoteAddr: "[2001:db8::1]:8080"},
		{name: "bad_port", method: http.MethodGet, target: "/hello/world", remoteAddr: "192.0.2.1:notaport"},
		{name: "port_out_of_range", method: http.MethodGet, target: "/hello/world", remoteAddr: "192.0.2.1:99999"},
		{name: "request_headers", method: http.MethodGet, target: "/hello/world", remoteAddr: "192.0.2.1:1234", headers: map[string]string{"X-Test": "value"}},
		{name: "status_and_headers", method: http.MethodGet, target: "/multi", remoteAddr: "192.0.2.1:1234"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			newReq := func() *http.Request {
				var body io.Reader
				if tt.body != nil {
					body = tt.body()
				}

				req := httptest.NewRequestWithContext(t.Context(), tt.method, tt.target, body)
				req.RemoteAddr = tt.remoteAddr
				for k, v := range tt.headers {
					req.Header.Set(k, v)
				}

				return req
			}

			want := httptest.NewRecorder()
			adaptor.FiberApp(adaptorEquivalenceApp())(want, newReq())

			got := httptest.NewRecorder()
			newHTTPHandler(adaptorEquivalenceApp())(got, newReq())

			if got.Code != want.Code {
				t.Fatalf("status = %d, want %d", got.Code, want.Code)
			}

			if !reflect.DeepEqual(got.Header(), want.Header()) {
				t.Fatalf("headers = %v, want %v", got.Header(), want.Header())
			}

			if got.Body.String() != want.Body.String() {
				t.Fatalf("body = %q, want %q", got.Body.String(), want.Body.String())
			}
		})
	}
}

// TestResolveRemoteAddr locks the literal-address fast path and the fallback
// error behavior to the strings net.ResolveTCPAddr would produce.
func TestResolveRemoteAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addr    string
		want    string
		wantErr bool
	}{
		{name: "ipv4", addr: "192.0.2.1:1234", want: "192.0.2.1:1234"},
		{name: "ipv4_missing_port", addr: "192.0.2.1", want: "192.0.2.1:80"},
		{name: "ipv4_zero_port", addr: "192.0.2.1:0", want: "192.0.2.1:0"},
		{name: "ipv6", addr: "[2001:db8::1]:8080", want: "[2001:db8::1]:8080"},
		{name: "port_out_of_range", addr: "192.0.2.1:99999", wantErr: true},
		{name: "port_not_numeric", addr: "192.0.2.1:notaport", wantErr: true},
		{name: "too_many_colons", addr: "1:2:3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveRemoteAddr(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveRemoteAddr(%q) = %v, want error", tt.addr, got)
				}

				return
			}

			if err != nil {
				t.Fatalf("resolveRemoteAddr(%q) error = %v", tt.addr, err)
			}

			if _, ok := got.(*net.TCPAddr); !ok {
				t.Fatalf("resolveRemoteAddr(%q) type = %T, want *net.TCPAddr", tt.addr, got)
			}

			if got.String() != tt.want {
				t.Fatalf("resolveRemoteAddr(%q) = %q, want %q", tt.addr, got.String(), tt.want)
			}
		})
	}
}
