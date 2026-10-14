package shop_test

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
)

// pagingProbe records the cursor and limit the generated gRPC adapter hands
// to ListProducts; every other ShopService method is left nil because the
// test never reaches it.
type pagingProbe struct {
	genshop.ShopService

	cursor string
	limit  int32
	called bool
}

func (p *pagingProbe) ListProducts(_ context.Context, _ *genshop.ListProductsRequest, cursor string, limit int32) (*genshop.ListProductsResponse, error) {
	p.cursor, p.limit, p.called = cursor, limit, true

	return &genshop.ListProductsResponse{}, nil
}

// TestGRPCListProductsPassesCursorAndClampsLimit proves a gRPC caller can
// paginate: the request's cursor reaches the service untouched, and the
// limit is normalized by the same bounds the HTTP router applies (zero
// becomes the default, oversize is clamped, negative is InvalidArgument).
func TestGRPCListProductsPassesCursorAndClampsLimit(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		limit     int32
		wantLimit int32
		wantCode  codes.Code
	}{
		{"zero uses default", 0, 20, codes.OK},
		{"in range passes through", 7, 7, codes.OK},
		{"oversize is clamped", 5000, 100, codes.OK},
		{"negative is rejected", -1, 0, codes.InvalidArgument},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			probe := &pagingProbe{}
			srv := genshop.NewShopServiceGRPCServer(probe)

			_, err := srv.ListProducts(t.Context(), &genshop.ListProductsRequest{Cursor: "40", Limit: tc.limit})
			if got := status.Code(err); got != tc.wantCode {
				t.Fatalf("code = %v, want %v (err %v)", got, tc.wantCode, err)
			}

			if tc.wantCode != codes.OK {
				if probe.called {
					t.Fatal("service called despite invalid limit")
				}

				return
			}

			if probe.cursor != "40" || probe.limit != tc.wantLimit {
				t.Fatalf("service saw cursor=%q limit=%d, want cursor=\"40\" limit=%d", probe.cursor, probe.limit, tc.wantLimit)
			}
		})
	}
}
