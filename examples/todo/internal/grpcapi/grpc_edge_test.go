package grpcapi_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	genapp "github.com/zenta-dev/zever/examples/todo/generated/gogen/app"
	pb "github.com/zenta-dev/zever/examples/todo/generated/protogogen"
	"github.com/zenta-dev/zever/examples/todo/internal/grpcapi"
	"github.com/zenta-dev/zever/examples/todo/internal/testsetup"
	"github.com/zenta-dev/zever/shared/apperror"
)

// edgeFixture wires a service with real auth for interceptor-level calls.
type edgeFixture struct {
	wiring *grpcapi.Wiring
	tokenA string
	tokenB string
}

func newEdgeFixture(t *testing.T) *edgeFixture {
	t.Helper()

	testsetup.RegisterDefaults()
	cfg := config.Default()
	cfg.Auth.Options.JWT.Secret = "test-secret-for-todo-edge-32-bytes-min!"
	c := container.New(cfg)
	t.Cleanup(func() { _ = c.Close(t.Context()) })
	authInst, err := c.Auth()
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	w, err := grpcapi.Build(authInst)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	tokA, err := authInst.Issue(t.Context(), "user-a", nil, time.Hour)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	tokB, err := authInst.Issue(t.Context(), "user-b", nil, time.Hour)
	if err != nil {
		t.Fatalf("Issue B: %v", err)
	}
	return &edgeFixture{wiring: w, tokenA: tokA.Value, tokenB: tokB.Value}
}

// call invokes method through the authz interceptor.
func (f *edgeFixture) call(method, token string, req any, handler grpc.UnaryHandler) (any, error) {
	ctx := context.Background()
	if token != "" {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	}
	return f.wiring.Interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, handler)
}

// codeString normalizes interceptor status errors and service apperror
// errors onto one comparable code name.
func codeString(err error) string {
	if err == nil {
		return ""
	}
	if st, ok := status.FromError(err); ok && st.Code() != codes.Unknown {
		return strings.ToUpper(st.Code().String())
	}
	var ae *apperror.Error
	if errors.As(err, &ae) {
		return ae.Code().String()
	}
	return "UNKNOWN"
}

func createHandler(svc *grpcapi.Service) grpc.UnaryHandler {
	return func(ctx context.Context, req any) (any, error) {
		if req == nil {
			return svc.CreateTask(ctx, nil)
		}
		typed, ok := req.(*genapp.CreateTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.CreateTask(ctx, typed)
	}
}

// TestServiceUnauthenticated pins unauthenticated errors for every method.
func TestServiceUnauthenticated(t *testing.T) {
	f := newEdgeFixture(t)
	svc := f.wiring.Service

	if _, err := f.call(pb.GrpcTaskService_CreateTask_FullMethodName, "", &genapp.CreateTaskRequest{Title: "x"}, createHandler(svc)); codeString(err) != apperror.Unauthenticated.String() {
		t.Errorf("CreateTask anon = %v, want Unauthenticated", err)
	}
	getHandler := func(ctx context.Context, req any) (any, error) {
		typed, ok := req.(*genapp.GetTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.GetTask(ctx, typed)
	}
	if _, err := f.call(pb.GrpcTaskService_GetTask_FullMethodName, "", &genapp.GetTaskRequest{Id: "x"}, getHandler); codeString(err) != apperror.Unauthenticated.String() {
		t.Errorf("GetTask anon = %v, want Unauthenticated", err)
	}
	delHandler := func(ctx context.Context, req any) (any, error) {
		typed, ok := req.(*genapp.DeleteTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.DeleteTask(ctx, typed)
	}
	if _, err := f.call(pb.GrpcTaskService_DeleteTask_FullMethodName, "", &genapp.DeleteTaskRequest{Id: "x"}, delHandler); codeString(err) != apperror.Unauthenticated.String() {
		t.Errorf("DeleteTask anon = %v, want Unauthenticated", err)
	}
}

// TestServiceValidation pins invalid-argument for nil/empty requests.
func TestServiceValidation(t *testing.T) {
	f := newEdgeFixture(t)
	svc := f.wiring.Service
	h := createHandler(svc)

	if _, err := f.call(pb.GrpcTaskService_CreateTask_FullMethodName, f.tokenA, nil, h); codeString(err) != apperror.InvalidArgument.String() {
		t.Errorf("CreateTask nil = %v, want InvalidArgument", err)
	}
	if _, err := f.call(pb.GrpcTaskService_CreateTask_FullMethodName, f.tokenA, &genapp.CreateTaskRequest{Title: "  "}, h); codeString(err) != apperror.InvalidArgument.String() {
		t.Errorf("CreateTask empty title = %v, want InvalidArgument", err)
	}
	getHandler := func(ctx context.Context, req any) (any, error) {
		typed, ok := req.(*genapp.GetTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.GetTask(ctx, typed)
	}
	if _, err := f.call(pb.GrpcTaskService_GetTask_FullMethodName, f.tokenA, &genapp.GetTaskRequest{}, getHandler); codeString(err) != apperror.InvalidArgument.String() && codeString(err) != apperror.NotFound.String() {
		t.Errorf("GetTask empty id = %v, want InvalidArgument/NotFound", err)
	}
}

// TestServiceUnknownIDs pins not-found for missing tasks.
func TestServiceUnknownIDs(t *testing.T) {
	f := newEdgeFixture(t)
	svc := f.wiring.Service
	getHandler := func(ctx context.Context, req any) (any, error) {
		typed, ok := req.(*genapp.GetTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.GetTask(ctx, typed)
	}
	delHandler := func(ctx context.Context, req any) (any, error) {
		typed, ok := req.(*genapp.DeleteTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.DeleteTask(ctx, typed)
	}
	if _, err := f.call(pb.GrpcTaskService_GetTask_FullMethodName, f.tokenA, &genapp.GetTaskRequest{Id: "missing"}, getHandler); codeString(err) != apperror.NotFound.String() {
		t.Errorf("GetTask missing = %v, want NotFound", err)
	}
	if _, err := f.call(pb.GrpcTaskService_DeleteTask_FullMethodName, f.tokenA, &genapp.DeleteTaskRequest{Id: "missing"}, delHandler); codeString(err) != apperror.NotFound.String() {
		t.Errorf("DeleteTask missing = %v, want NotFound", err)
	}
}

// TestServiceOwnerScoping pins cross-owner get/delete decisions.
func TestServiceOwnerScoping(t *testing.T) {
	f := newEdgeFixture(t)
	svc := f.wiring.Service

	createdAny, err := f.call(pb.GrpcTaskService_CreateTask_FullMethodName, f.tokenA, &genapp.CreateTaskRequest{Title: "private", Body: "b"}, createHandler(svc))
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	created, ok := createdAny.(*genapp.GrpcTask)
	if !ok {
		t.Fatalf("CreateTask type = %T, want *GrpcTask", createdAny)
	}
	getHandler := func(ctx context.Context, req any) (any, error) {
		typed, ok := req.(*genapp.GetTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.GetTask(ctx, typed)
	}
	delHandler := func(ctx context.Context, req any) (any, error) {
		typed, ok := req.(*genapp.DeleteTaskRequest)
		if !ok {
			return nil, errors.New("bad request type")
		}
		return svc.DeleteTask(ctx, typed)
	}
	if _, err := f.call(pb.GrpcTaskService_GetTask_FullMethodName, f.tokenB, &genapp.GetTaskRequest{Id: created.Id}, getHandler); codeString(err) != apperror.NotFound.String() {
		t.Errorf("GetTask foreign = %v, want NotFound", err)
	}
	if _, err := f.call(pb.GrpcTaskService_DeleteTask_FullMethodName, f.tokenB, &genapp.DeleteTaskRequest{Id: created.Id}, delHandler); codeString(err) != apperror.PermissionDenied.String() {
		t.Errorf("DeleteTask foreign = %v, want PermissionDenied", err)
	}
}

// TestServiceConcurrentCreate pins goroutine-safe creation.
func TestServiceConcurrentCreate(t *testing.T) {
	svc := grpcapi.New()
	f := newEdgeFixture(t)

	const n = 16
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = f.call(pb.GrpcTaskService_CreateTask_FullMethodName, f.tokenA, &genapp.CreateTaskRequest{Title: "t"}, createHandler(svc))
		}(i)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatalf("CreateTask concurrent: %v", err)
		}
	}
}
