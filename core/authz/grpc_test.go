package authz_test

import (
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/zenta-dev/zever/auth"
	"github.com/zenta-dev/zever/authz"
)

type echoSrv struct {
	claims auth.Claims
	has    bool
}

//nolint:unparam // error result required by echoServer iface mirrors grpc handler shape; always nil in tests.
func (s *echoSrv) echo(ctx context.Context, in *wrapperspb.StringValue) (*wrapperspb.StringValue, error) {
	c, ok := authz.ClaimsFromContext(ctx)
	s.claims, s.has = c, ok
	return &wrapperspb.StringValue{Value: "ok:" + in.Value}, nil
}

type echoServer interface {
	echo(context.Context, *wrapperspb.StringValue) (*wrapperspb.StringValue, error)
}

func serveEcho(t *testing.T, a *fakeAuth, p *fakeChecker, policies map[string]authz.Policy) (string, *echoSrv) {
	t.Helper()

	srv := &echoSrv{}
	s := grpc.NewServer(grpc.UnaryInterceptor(authz.UnaryServerInterceptor(a, p, policies)))
	s.RegisterService(&grpc.ServiceDesc{
		ServiceName: "test.Test",
		HandlerType: (*echoServer)(nil),
		Methods: []grpc.MethodDesc{{
			MethodName: "Echo",
			Handler: func(srvAny any, ctx context.Context, dec func(any) error, interceptor grpc.UnaryServerInterceptor) (any, error) {
				in := new(wrapperspb.StringValue)
				if err := dec(in); err != nil {
					return nil, err
				}
				srvTyped, ok := srvAny.(*echoSrv)
				if !ok {
					return nil, status.Errorf(codes.Internal, "bad server type %T", srvAny)
				}
				if interceptor == nil {
					return srvTyped.echo(ctx, in)
				}
				info := &grpc.UnaryServerInfo{Server: srvAny, FullMethod: "/test.Test/Echo"}
				handler := func(ctx context.Context, req any) (any, error) {
					inReq, ok := req.(*wrapperspb.StringValue)
					if !ok {
						return nil, status.Errorf(codes.Internal, "bad request type %T", req)
					}
					return srvTyped.echo(ctx, inReq)
				}
				return interceptor(ctx, in, info, handler)
			},
		}},
	}, srv)

	lis, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = s.Serve(lis) }()
	t.Cleanup(s.Stop)
	return lis.Addr().String(), srv
}

func echoCall(t *testing.T, addr string, md metadata.MD) (*wrapperspb.StringValue, error) {
	t.Helper()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	ctx := t.Context()
	if md != nil {
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	out := new(wrapperspb.StringValue)
	err = conn.Invoke(ctx, "/test.Test/Echo", &wrapperspb.StringValue{Value: "hi"}, out)
	return out, err
}

func TestGRPCMissPassthrough(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{}
	p := &fakeChecker{}
	addr, _ := serveEcho(t, a, p, map[string]authz.Policy{})

	out, err := echoCall(t, addr, nil)
	if err != nil {
		t.Fatalf("Invoke() err = %v, want nil", err)
	}
	if out.Value != "ok:hi" {
		t.Fatalf("Invoke() = %q, want ok:hi", out.Value)
	}
	if a.called || p.called {
		t.Fatal("miss must not call Authorize backends")
	}
}

func TestGRPCNoMetadataUnauthenticated(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{}
	addr, _ := serveEcho(t, a, p, map[string]authz.Policy{
		"/test.Test/Echo": {AuthRequired: true},
	})

	_, err := echoCall(t, addr, nil)
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Code() = %v, want Unauthenticated (err=%v)", status.Code(err), err)
	}
}

func TestGRPCInvalidToken(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good"}
	p := &fakeChecker{}
	addr, _ := serveEcho(t, a, p, map[string]authz.Policy{
		"/test.Test/Echo": {AuthRequired: true},
	})

	_, err := echoCall(t, addr, metadata.Pairs("authorization", "Bearer bad"))
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("Code() = %v, want Unauthenticated (err=%v)", status.Code(err), err)
	}
}

func TestGRPCDenied(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: false}
	addr, _ := serveEcho(t, a, p, map[string]authz.Policy{
		"/test.Test/Echo": {AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"},
	})

	_, err := echoCall(t, addr, metadata.Pairs("authorization", "Bearer good"))
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("Code() = %v, want PermissionDenied (err=%v)", status.Code(err), err)
	}
}

func TestGRPCSuccessClaimsVisible(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "user-3"}}
	p := &fakeChecker{allow: true}
	addr, srv := serveEcho(t, a, p, map[string]authz.Policy{
		"/test.Test/Echo": {AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"},
	})

	out, err := echoCall(t, addr, metadata.Pairs("authorization", "Bearer good"))
	if err != nil {
		t.Fatalf("Invoke() err = %v, want nil", err)
	}
	if out.Value != "ok:hi" {
		t.Fatalf("Invoke() = %q, want ok:hi", out.Value)
	}
	if !srv.has || srv.claims.Subject != "user-3" {
		t.Fatalf("handler claims = (%+v, %v), want (user-3, true)", srv.claims, srv.has)
	}
}

func TestBearerTokenFromMD(t *testing.T) {
	t.Parallel()

	ctx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer abc"))
	if got := authz.BearerTokenFromMD(ctx); got != "abc" {
		t.Fatalf("BearerTokenFromMD() = %q, want abc", got)
	}
	if got := authz.BearerTokenFromMD(t.Context()); got != "" {
		t.Fatalf("BearerTokenFromMD() no-md = %q, want empty", got)
	}
	multi := metadata.NewIncomingContext(t.Context(), metadata.MD{"authorization": {"Bearer abc", "Bearer def"}})
	if got := authz.BearerTokenFromMD(multi); got != "" {
		t.Fatalf("BearerTokenFromMD() multi = %q, want empty (fail closed)", got)
	}
}

type idRequest struct{ id string }

// GetId implements authz.ResourceIDer, mirroring protogogen Get-by-id messages.
//
//nolint:revive // GetId (not GetID) matches the protobuf getter protoc-gen-go emits for an id field.
func (r idRequest) GetId() string { return r.id }

// TestGRPCResourceIDFromRequest proves the interceptor populates the
// permission check's resource id from requests implementing ResourceIDer,
// and leaves it empty otherwise.
func TestGRPCResourceIDFromRequest(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{wantToken: "good", claims: auth.Claims{Subject: "u"}}
	p := &fakeChecker{allow: true}
	policies := map[string]authz.Policy{
		"/test.Test/Echo": {AuthRequired: true, PermissionCheck: "x.y", ResourceType: "T"},
	}
	iv := authz.UnaryServerInterceptor(a, p, policies)
	mdCtx := metadata.NewIncomingContext(t.Context(), metadata.Pairs("authorization", "Bearer good"))
	info := &grpc.UnaryServerInfo{FullMethod: "/test.Test/Echo"}
	handler := func(context.Context, any) (any, error) { return struct{}{}, nil }

	if _, err := iv(mdCtx, idRequest{id: "r1"}, info, handler); err != nil {
		t.Fatalf("Invoke() err = %v, want nil", err)
	}
	if !p.called || p.gotResource.ID != "r1" || p.gotResource.Type != "T" {
		t.Fatalf("resource = %+v, want {Type:T ID:r1}", p.gotResource)
	}

	p.called = false
	if _, err := iv(mdCtx, &wrapperspb.StringValue{Value: "hi"}, info, handler); err != nil {
		t.Fatalf("Invoke() err = %v, want nil", err)
	}
	if !p.called || p.gotResource.ID != "" {
		t.Fatalf("resource = %+v, want empty ID", p.gotResource)
	}
}
