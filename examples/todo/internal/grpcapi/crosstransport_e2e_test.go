package grpcapi_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	genapp "github.com/zenta-dev/zever/examples/todo/generated/gogen/app"
	pb "github.com/zenta-dev/zever/examples/todo/generated/protogogen"
	"github.com/zenta-dev/zever/examples/todo/internal/grpcapi"

	_ "github.com/zenta-dev/zever/auth/jwt"
	_ "github.com/zenta-dev/zever/cache/memory"
	_ "github.com/zenta-dev/zever/db/sqlite"
	_ "github.com/zenta-dev/zever/log/slog"
	_ "github.com/zenta-dev/zever/password/argon2"
	_ "github.com/zenta-dev/zever/queue/memory"
	_ "github.com/zenta-dev/zever/router/stdhttp"
)

const testJWTSecret = "test-secret-for-todo-grpc-32-bytes-min!"

// fixture wires one shared service to HTTP routes (same generated policies)
// and the gRPC interceptor built from GRPCPolicies().
type fixture struct {
	http   http.Handler
	wiring *grpcapi.Wiring
	token  string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	cfg := config.Default()
	cfg.Auth.Options.JWT.Secret = testJWTSecret

	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(context.Background()) })

	authInst, err := c.Auth()
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}

	w, err := grpcapi.Build(authInst)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	genapp.RegisterGrpcTaskServiceRoutes(r, w.Service, authInst, w.Checker)

	token, err := authInst.Issue(context.Background(), "user-a", nil, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	return &fixture{http: r, wiring: w, token: token.Value}
}

// grpcCall invokes the wired interceptor for method with an optional token.
func (f *fixture) grpcCall(method string, token string, req any, handler grpc.UnaryHandler) (any, error) {
	ctx := context.Background()
	if token != "" {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	}
	return f.wiring.Interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, handler)
}

// httpPost posts JSON to the HTTP transport with an optional token.
func httpPost(t *testing.T, h http.Handler, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type taskJSON struct {
	ID     string `json:"id"`
	UserID string `json:"userId"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// TestCreateIdenticalAcrossTransports proves the same input creates the
// same shaped task on both transports.
func TestCreateIdenticalAcrossTransports(t *testing.T) {
	f := newFixture(t)

	rec := httpPost(t, f.http, "/grpc-tasks", f.token, `{"title":"buy milk","body":"2%"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("http create: got %d body %s", rec.Code, rec.Body.String())
	}
	var httpTask taskJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &httpTask); err != nil {
		t.Fatalf("http decode: %v", err)
	}

	resp, err := f.grpcCall(pb.GrpcTaskService_CreateTask_FullMethodName, f.token,
		&genapp.CreateTaskRequest{Title: "buy milk", Body: "2%"},
		func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*genapp.CreateTaskRequest)
			if !ok {
				t.Fatalf("create req type = %T", req)
			}
			return f.wiring.Service.CreateTask(ctx, typed)
		})
	if err != nil {
		t.Fatalf("grpc create: %v", err)
	}
	grpcTask, ok := resp.(*genapp.GrpcTask)
	if !ok {
		t.Fatalf("grpc create type = %T", resp)
	}

	if grpcTask.Title != httpTask.Title || grpcTask.Body != httpTask.Body || grpcTask.UserId != httpTask.UserID {
		t.Fatalf("transports differ: http %+v grpc id=%s user=%s title=%s body=%s",
			httpTask, grpcTask.Id, grpcTask.UserId, grpcTask.Title, grpcTask.Body)
	}
	if grpcTask.Id == "" || httpTask.ID == "" {
		t.Fatalf("missing ids: http %+v grpc %s", httpTask, grpcTask.Id)
	}
}

// TestUnauthenticatedRejectedIdentically proves no token fails the same on
// both transports: HTTP 401, gRPC Unauthenticated.
func TestUnauthenticatedRejectedIdentically(t *testing.T) {
	f := newFixture(t)

	rec := httpPost(t, f.http, "/grpc-tasks", "", `{"title":"x","body":"y"}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("http unauthed: got %d body %s", rec.Code, rec.Body.String())
	}

	_, err := f.grpcCall(pb.GrpcTaskService_CreateTask_FullMethodName, "",
		&genapp.CreateTaskRequest{Title: "x", Body: "y"},
		func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*genapp.CreateTaskRequest)
			if !ok {
				t.Fatalf("create req type = %T", req)
			}
			return f.wiring.Service.CreateTask(ctx, typed)
		})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("grpc unauthed: got %v", err)
	}
}

// TestUnauthorizedRejectedIdentically proves an authenticated caller
// without permission fails the same on both transports: HTTP 403, gRPC
// PermissionDenied. The shared rbac checker carries no allow rule for
// "grpc_task.delete", so DeleteTask denies every caller on both sides.
func TestUnauthorizedRejectedIdentically(t *testing.T) {
	f := newFixture(t)

	rec := httpPost(t, f.http, "/grpc-tasks", f.token, `{"title":"t","body":"b"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("http create: got %d", rec.Code)
	}
	var created taskJSON
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	delPath := "/grpc-tasks/" + created.ID
	req := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, delPath, nil)
	req.Header.Set("Authorization", "Bearer "+f.token)
	rec = httptest.NewRecorder()
	f.http.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("http delete: got %d body %s", rec.Code, rec.Body.String())
	}

	_, err := f.grpcCall(pb.GrpcTaskService_DeleteTask_FullMethodName, f.token,
		&genapp.DeleteTaskRequest{Id: created.ID},
		func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*genapp.DeleteTaskRequest)
			if !ok {
				t.Fatalf("delete req type = %T", req)
			}
			return f.wiring.Service.DeleteTask(ctx, typed)
		})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("grpc delete: got %v", err)
	}
}
