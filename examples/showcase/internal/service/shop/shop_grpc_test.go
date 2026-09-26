package shop_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/zenta-dev/zever/adapters/permission/rbac"
	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/seed"
	shopsvc "github.com/zenta-dev/zever/examples/showcase/internal/service/shop"
	"github.com/zenta-dev/zever/examples/showcase/internal/testsetup"

	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
	pb "github.com/zenta-dev/zever/examples/showcase/generated/protogogen/shop"
)

// grpcParityJWTSecret signs the tokens this fixture issues.
const grpcParityJWTSecret = "test-secret-for-showcase-32-bytes-min"

// grpcParityFixture wires one shared ShopServiceImpl to HTTP routes (same
// generated policies) and the gRPC interceptor built from GRPCPolicies().
type grpcParityFixture struct {
	impl        *shopsvc.ShopServiceImpl
	grpc        *genshop.ShopServiceGRPCServer
	interceptor grpc.UnaryServerInterceptor
	http        http.Handler
	auth        auth.Auth
	queue       queue.Queue
	database    db.DB
}

// newGRPCFixture builds a fresh sqlite-backed service per test.
func newGRPCFixture(t *testing.T) *grpcParityFixture {
	t.Helper()

	testsetup.RegisterDefaults()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Auth.Options.JWT.Secret = grpcParityJWTSecret

	c := container.New(cfg)

	ctx := context.Background()
	database, err := c.DB()
	if err != nil {
		t.Fatalf("DB: %v", err)
	}
	authInst, err := c.Auth()
	if err != nil {
		t.Fatalf("Auth: %v", err)
	}
	q, err := c.Queue()
	if err != nil {
		t.Fatalf("Queue: %v", err)
	}
	r, err := c.Router()
	if err != nil {
		t.Fatalf("Router: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, execErr := database.Exec(ctx, stmt); execErr != nil {
			t.Fatalf("migrate: %v", execErr)
		}
	}
	if joinErr := seed.EnsureJoinTable(ctx, database); joinErr != nil {
		t.Fatalf("join table: %v", joinErr)
	}

	checker, err := rbac.New(permission.Options{})
	if err != nil {
		t.Fatalf("checker: %v", err)
	}

	impl := shopsvc.NewShopServiceImpl(shopsvc.Deps{DB: database, Queue: q})
	genshop.RegisterShopServiceRoutes(r, impl, authInst, checker)

	t.Cleanup(func() {
		_ = c.Close(t.Context())
	})

	return &grpcParityFixture{
		impl:        impl,
		grpc:        genshop.NewShopServiceGRPCServer(impl),
		interceptor: authz.UnaryServerInterceptor(authInst, checker, genshop.GRPCPolicies()),
		http:        r,
		auth:        authInst,
		queue:       q,
		database:    database,
	}
}

// grpcCall invokes the wired interceptor for method with an optional token.
func (f *grpcParityFixture) grpcCall(method string, token string, req any, handler grpc.UnaryHandler) (any, error) {
	ctx := context.Background()
	if token != "" {
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs("authorization", "Bearer "+token))
	}
	return f.interceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, handler)
}

// grpcParityToken issues a bearer token for subject.
func (f *grpcParityFixture) grpcParityToken(t *testing.T, subject string) string {
	t.Helper()

	tok, err := f.auth.Issue(t.Context(), subject, nil, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return tok.Value
}

// grpcParityHTTP sends a raw request to the HTTP transport.
func (f *grpcParityFixture) grpcParityHTTP(method, path, token, body string) *httptest.ResponseRecorder {
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequestWithContext(context.Background(), method, path, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	f.http.ServeHTTP(rec, req)
	return rec
}

// TestShopGRPCProductParity proves CreateProduct plus GetProduct return
// identical data on both transports.
func TestShopGRPCProductParity(t *testing.T) {
	f := newGRPCFixture(t)
	token := f.grpcParityToken(t, "user-1")

	rec := f.grpcParityHTTP(http.MethodPost, "/products", token, `{"name":"http-widget","priceCents":2500}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("http create: got %d body %s", rec.Code, rec.Body.String())
	}
	var httpCreated genshop.Product
	if err := protojson.Unmarshal(rec.Body.Bytes(), &httpCreated); err != nil {
		t.Fatalf("http decode: %v", err)
	}

	resp, err := f.grpcCall(pb.ShopService_CreateProduct_FullMethodName, token,
		&genshop.CreateProductRequest{Name: "grpc-widget", PriceCents: 2500},
		func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*genshop.CreateProductRequest)
			if !ok {
				t.Fatalf("create req type = %T", req)
			}
			return f.grpc.CreateProduct(ctx, typed)
		})
	if err != nil {
		t.Fatalf("grpc create: %v", err)
	}
	grpcCreated, ok := resp.(*genshop.Product)
	if !ok {
		t.Fatalf("grpc create type = %T", resp)
	}

	if httpCreated.GetHeadline() != "http-widget" || grpcCreated.GetHeadline() != "grpc-widget" {
		t.Fatalf("headlines differ: http %q grpc %q", httpCreated.GetHeadline(), grpcCreated.GetHeadline())
	}
	if httpCreated.GetPriceCents() != grpcCreated.GetPriceCents() || httpCreated.GetPriceCents() != 2500 {
		t.Fatalf("prices differ: http %d grpc %d", httpCreated.GetPriceCents(), grpcCreated.GetPriceCents())
	}
	if httpCreated.GetCategoryId() != grpcCreated.GetCategoryId() || httpCreated.GetCategoryId() == "" {
		t.Fatalf("categories differ: http %q grpc %q", httpCreated.GetCategoryId(), grpcCreated.GetCategoryId())
	}
	if httpCreated.GetId() == "" || grpcCreated.GetId() == "" {
		t.Fatalf("missing ids: http %q grpc %q", httpCreated.GetId(), grpcCreated.GetId())
	}

	// GetProduct round-trips each created row on its own transport.
	httpGet := f.grpcParityHTTP(http.MethodGet, "/products/"+httpCreated.GetId(), "", "")
	if httpGet.Code != http.StatusOK {
		t.Fatalf("http get: got %d body %s", httpGet.Code, httpGet.Body.String())
	}
	var httpGot genshop.Product
	if decErr := protojson.Unmarshal(httpGet.Body.Bytes(), &httpGot); decErr != nil {
		t.Fatalf("http get decode: %v", decErr)
	}
	if httpGot.GetId() != httpCreated.GetId() || httpGot.GetHeadline() != httpCreated.GetHeadline() {
		t.Fatalf("http get = %+v, want created %+v", &httpGot, &httpCreated)
	}

	grpcGetResp, err := f.grpcCall(pb.ShopService_GetProduct_FullMethodName, "",
		&genshop.GetProductRequest{Id: grpcCreated.GetId()},
		func(ctx context.Context, req any) (any, error) {
			typed, convOk := req.(*genshop.GetProductRequest)
			if !convOk {
				t.Fatalf("get req type = %T", req)
			}
			return f.grpc.GetProduct(ctx, typed)
		})
	if err != nil {
		t.Fatalf("grpc get: %v", err)
	}
	grpcGot, ok := grpcGetResp.(*genshop.Product)
	if !ok {
		t.Fatalf("grpc get type = %T", grpcGetResp)
	}
	if grpcGot.GetId() != grpcCreated.GetId() || grpcGot.GetHeadline() != grpcCreated.GetHeadline() {
		t.Fatalf("grpc get id=%q headline=%q, want id=%q headline=%q",
			grpcGot.GetId(), grpcGot.GetHeadline(), grpcCreated.GetId(), grpcCreated.GetHeadline())
	}
}

// TestShopGRPCUnauthenticatedRejected proves no token fails the same on
// both transports: HTTP 401, gRPC Unauthenticated.
func TestShopGRPCUnauthenticatedRejected(t *testing.T) {
	f := newGRPCFixture(t)

	rec := f.grpcParityHTTP(http.MethodPost, "/products", "", `{"name":"x","priceCents":1}`)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("http unauthed: got %d body %s", rec.Code, rec.Body.String())
	}

	_, err := f.grpcCall(pb.ShopService_CreateProduct_FullMethodName, "",
		&genshop.CreateProductRequest{Name: "x", PriceCents: 1},
		func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*genshop.CreateProductRequest)
			if !ok {
				t.Fatalf("create req type = %T", req)
			}
			return f.grpc.CreateProduct(ctx, typed)
		})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("grpc unauthed: got %v", err)
	}
}

// grpcParityInsertUser adds a member row to the fixture database.
func grpcParityInsertUser(t *testing.T, f *grpcParityFixture, id, email string) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := f.database.Exec(t.Context(),
		`INSERT INTO users (id, email, name, nickname, role, password_hash, age, credit_cents, rating, score, verified, birthday, avatar, prefs, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, email, "Test User", nil, "member", "hash", 30, 0, 0.0, 0.0, 0,
		"1990-01-01T00:00:00Z", []byte{}, "{}", now); err != nil {
		t.Fatalf("user: %v", err)
	}
}

// grpcParityInsertOrder adds a pending low-priority order row to the fixture database.
func grpcParityInsertOrder(t *testing.T, f *grpcParityFixture, id, userID string, total int64) {
	t.Helper()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := f.database.Exec(t.Context(),
		`INSERT INTO orders (id, user_id, total_cents, status, priority, note, created_at) VALUES (?, ?, ?, 'pending', 'low', ?, ?)`,
		id, userID, total, nil, now); err != nil {
		t.Fatalf("order: %v", err)
	}
}

// TestShopGRPCGetOrderOwnerParity proves the owner reads the order on both
// transports while another caller is denied on both: HTTP 403, gRPC
// PermissionDenied.
func TestShopGRPCGetOrderOwnerParity(t *testing.T) {
	f := newGRPCFixture(t)
	grpcParityInsertUser(t, f, "user-1", "one@example.com")
	grpcParityInsertUser(t, f, "user-2", "two@example.com")
	grpcParityInsertOrder(t, f, "order-1", "user-1", 1200)
	owner := f.grpcParityToken(t, "user-1")
	other := f.grpcParityToken(t, "user-2")

	rec := f.grpcParityHTTP(http.MethodGet, "/orders/order-1", owner, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("http owner get: got %d body %s", rec.Code, rec.Body.String())
	}
	var httpOrder genshop.Order
	if err := protojson.Unmarshal(rec.Body.Bytes(), &httpOrder); err != nil {
		t.Fatalf("http decode: %v", err)
	}

	resp, err := f.grpcCall(pb.ShopService_GetOrder_FullMethodName, owner,
		&genshop.GetOrderRequest{Id: "order-1"},
		func(ctx context.Context, req any) (any, error) {
			typed, convOk := req.(*genshop.GetOrderRequest)
			if !convOk {
				t.Fatalf("get order req type = %T", req)
			}
			return f.grpc.GetOrder(ctx, typed)
		})
	if err != nil {
		t.Fatalf("grpc owner get: %v", err)
	}
	grpcOrder, ok := resp.(*genshop.Order)
	if !ok {
		t.Fatalf("grpc get order type = %T", resp)
	}

	if httpOrder.GetId() != grpcOrder.GetId() || httpOrder.GetTotalCents() != grpcOrder.GetTotalCents() {
		t.Fatalf("transports differ: http id=%q total=%d grpc id=%q total=%d",
			httpOrder.GetId(), httpOrder.GetTotalCents(), grpcOrder.GetId(), grpcOrder.GetTotalCents())
	}
	if grpcOrder.GetId() != "order-1" || grpcOrder.GetTotalCents() != 1200 || grpcOrder.GetUserId() != "user-1" {
		t.Fatalf("order id=%q user=%q total=%d, want order-1/user-1/1200",
			grpcOrder.GetId(), grpcOrder.GetUserId(), grpcOrder.GetTotalCents())
	}

	denied := f.grpcParityHTTP(http.MethodGet, "/orders/order-1", other, "")
	if denied.Code != http.StatusForbidden {
		t.Fatalf("http non-owner: got %d body %s", denied.Code, denied.Body.String())
	}

	_, err = f.grpcCall(pb.ShopService_GetOrder_FullMethodName, other,
		&genshop.GetOrderRequest{Id: "order-1"},
		func(ctx context.Context, req any) (any, error) {
			typed, convOk := req.(*genshop.GetOrderRequest)
			if !convOk {
				t.Fatalf("get order req type = %T", req)
			}
			return f.grpc.GetOrder(ctx, typed)
		})
	if status.Code(err) != codes.PermissionDenied {
		t.Fatalf("grpc non-owner: got %v", err)
	}
}

// TestShopGRPCCheckout proves Checkout over gRPC creates a pending order
// and returns a receipt carrying the total.
func TestShopGRPCCheckout(t *testing.T) {
	f := newGRPCFixture(t)
	grpcParityInsertUser(t, f, "user-1", "one@example.com")
	owner := f.grpcParityToken(t, "user-1")

	resp, err := f.grpcCall(pb.ShopService_Checkout_FullMethodName, owner,
		&genshop.CheckoutRequest{
			Order:    &pb.CreateOrderRequest{UserId: "user-1", TotalCents: 5000},
			GiftNote: "hi",
		},
		func(ctx context.Context, req any) (any, error) {
			typed, ok := req.(*genshop.CheckoutRequest)
			if !ok {
				t.Fatalf("checkout req type = %T", req)
			}
			return f.grpc.Checkout(ctx, typed)
		})
	if err != nil {
		t.Fatalf("grpc checkout: %v", err)
	}
	receipt, ok := resp.(*genshop.OrderReceipt)
	if !ok {
		t.Fatalf("grpc checkout type = %T", resp)
	}
	if receipt.GetOrderId() == "" || receipt.GetTotalCents() != 5000 {
		t.Fatalf("receipt order=%q total=%d, want id + 5000", receipt.GetOrderId(), receipt.GetTotalCents())
	}

	gotResp, err := f.grpcCall(pb.ShopService_GetOrder_FullMethodName, owner,
		&genshop.GetOrderRequest{Id: receipt.GetOrderId()},
		func(ctx context.Context, req any) (any, error) {
			typed, convOk := req.(*genshop.GetOrderRequest)
			if !convOk {
				t.Fatalf("get order req type = %T", req)
			}
			return f.grpc.GetOrder(ctx, typed)
		})
	if err != nil {
		t.Fatalf("grpc get order: %v", err)
	}
	got, ok := gotResp.(*genshop.Order)
	if !ok {
		t.Fatalf("grpc get order type = %T", gotResp)
	}
	if got.GetStatus() != pb.OrderStatus_ORDER_STATUS_PENDING {
		t.Fatalf("status = %v, want pending", got.GetStatus())
	}
	if got.GetTotalCents() != 5000 || got.GetUserId() != "user-1" {
		t.Fatalf("order user=%q total=%d, want user-1/5000", got.GetUserId(), got.GetTotalCents())
	}

	msg, err := f.queue.Pop(t.Context(), "orders.confirm")
	if err != nil {
		t.Fatalf("pop: %v", err)
	}
	if !strings.Contains(string(msg.Payload), receipt.GetOrderId()) {
		t.Fatalf("payload = %q, want order id", string(msg.Payload))
	}
}
