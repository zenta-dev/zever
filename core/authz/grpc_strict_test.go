package authz_test

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zenta-dev/zever/core/authz"
)

func callInterceptor(t *testing.T, iv grpc.UnaryServerInterceptor, method string) (any, *bool, error) {
	t.Helper()
	called := false
	handler := func(context.Context, any) (any, error) {
		called = true
		return "ok", nil
	}
	info := &grpc.UnaryServerInfo{FullMethod: method}
	res, err := iv(t.Context(), struct{}{}, info, handler)
	return res, &called, err
}

// TestStrictInterceptor_UnlistedDenied proves the strict interceptor denies
// methods without a policy instead of passing them to the handler.
func TestStrictInterceptor_UnlistedDenied(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{}
	p := &fakeChecker{}
	iv := authz.UnaryServerInterceptorStrict(a, p, map[string]authz.Policy{})

	_, called, err := callInterceptor(t, iv, "/test.Test/Echo")
	if err == nil {
		t.Fatal("strict Invoke() err = nil, want PermissionDenied")
	}
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("strict Code() = %v, want PermissionDenied (err=%v)", status.Code(err), err)
	}
	if *called {
		t.Fatal("strict miss must not call the handler")
	}
	if a.called || p.called {
		t.Fatal("strict miss must not call Authorize backends")
	}
}

// TestLegacyInterceptor_UnlistedAllowed locks in the legacy fail-open
// behavior: methods without a policy pass straight to the handler.
func TestLegacyInterceptor_UnlistedAllowed(t *testing.T) {
	t.Parallel()

	a := &fakeAuth{}
	p := &fakeChecker{}
	iv := authz.UnaryServerInterceptor(a, p, map[string]authz.Policy{})

	res, called, err := callInterceptor(t, iv, "/test.Test/Echo")
	if err != nil {
		t.Fatalf("legacy Invoke() err = %v, want nil", err)
	}
	if res != "ok" {
		t.Fatalf("legacy Invoke() = %v, want ok", res)
	}
	if !*called {
		t.Fatal("legacy miss must call the handler")
	}
	if a.called || p.called {
		t.Fatal("legacy miss must not call Authorize backends")
	}
}

// TestStrictInterceptor_ListedMatchesLegacy proves a listed method takes the
// same allow path under both interceptors.
func TestStrictInterceptor_ListedMatchesLegacy(t *testing.T) {
	t.Parallel()

	policies := map[string]authz.Policy{
		"/test.Test/Echo": {},
	}
	for name, iv := range map[string]grpc.UnaryServerInterceptor{
		"legacy": authz.UnaryServerInterceptor(&fakeAuth{}, &fakeChecker{}, policies),
		"strict": authz.UnaryServerInterceptorStrict(&fakeAuth{}, &fakeChecker{}, policies),
	} {
		iv := iv
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			res, called, err := callInterceptor(t, iv, "/test.Test/Echo")
			if err != nil {
				t.Fatalf("Invoke() err = %v, want nil", err)
			}
			if res != "ok" || !*called {
				t.Fatalf("Invoke() = (%v, called=%v), want (ok, true)", res, *called)
			}
		})
	}
}
