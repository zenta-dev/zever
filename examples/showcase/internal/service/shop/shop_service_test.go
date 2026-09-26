package shop_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zenta-dev/zever/config"
	"github.com/zenta-dev/zever/container"
	"github.com/zenta-dev/zever/core/auth"
	"github.com/zenta-dev/zever/core/authz"
	"github.com/zenta-dev/zever/core/db"
	"github.com/zenta-dev/zever/core/queue"
	"github.com/zenta-dev/zever/examples/showcase/internal/service/seed"
	shopsvc "github.com/zenta-dev/zever/examples/showcase/internal/service/shop"
	"github.com/zenta-dev/zever/examples/showcase/internal/testsetup"
	"github.com/zenta-dev/zever/shared/apperror"

	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
	pb "github.com/zenta-dev/zever/examples/showcase/generated/protogogen/shop"
)

// testSetup bundles the service with the infra tests assert against.
type testSetup struct {
	svc   *shopsvc.ShopServiceImpl
	auth  auth.Auth
	queue queue.Queue
	db    db.DB
}

// newTestSetup builds a fresh sqlite-backed service per test.
func newTestSetup(t *testing.T) testSetup {
	t.Helper()

	testsetup.RegisterDefaults()

	cfg := config.Default()
	cfg.DB.Options.Path = filepath.Join(t.TempDir(), "test.db")
	cfg.Auth.Options.JWT.Secret = "test-secret-for-showcase-32-bytes-min"

	c := container.New(cfg)

	ctx := t.Context()
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

	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "testdata", "schema.sql"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	for _, stmt := range strings.Split(string(raw), ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := database.Exec(ctx, stmt); err != nil {
			t.Fatalf("migrate: %v", err)
		}
	}
	if err := seed.EnsureJoinTable(ctx, database); err != nil {
		t.Fatalf("join table: %v", err)
	}

	t.Cleanup(func() {
		_ = c.Close(t.Context())
	})

	return testSetup{
		svc:   shopsvc.NewShopServiceImpl(shopsvc.Deps{DB: database, Queue: q}),
		auth:  authInst,
		queue: q,
		db:    database,
	}
}

// ctxFor returns a context carrying verified claims for subject, minted and
// injected through the real auth middleware.
func ctxFor(t *testing.T, s testSetup, subject string) context.Context {
	t.Helper()

	tok, err := s.auth.Issue(t.Context(), subject, nil, time.Hour)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	var out context.Context
	called := false
	next := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		called = true
		out = r.Context()
	})
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tok.Value)
	rec := httptest.NewRecorder()
	authz.Middleware(s.auth, nil, authz.Policy{AuthRequired: true}, nil)(next).ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusOK {
		t.Fatalf("middleware rejected valid token: status %d", rec.Code)
	}
	return out
}

// codeOf unwraps an apperror code or fails the test.
func codeOf(t *testing.T, err error) apperror.ErrorCode {
	t.Helper()
	var appErr *apperror.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("err is not apperror: %v", err)
	}
	return appErr.Code()
}

func insertCategory(t *testing.T, s testSetup, id, name string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(t.Context(),
		`INSERT INTO categories (id, name, description, created_at) VALUES (?, ?, ?, ?)`,
		id, name, name+" desc", now); err != nil {
		t.Fatalf("category: %v", err)
	}
}

func insertUser(t *testing.T, s testSetup, id, email string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(t.Context(),
		`INSERT INTO users (id, email, name, nickname, role, password_hash, age, credit_cents, rating, score, verified, birthday, avatar, prefs, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, email, "Test User", nil, "member", "hash", 30, 0, 0.0, 0.0, 0,
		"1990-01-01T00:00:00Z", []byte{}, "{}", now); err != nil {
		t.Fatalf("user: %v", err)
	}
}

func insertOrder(t *testing.T, s testSetup, id, userID string, total int64, created string) {
	t.Helper()
	if _, err := s.db.Exec(t.Context(),
		`INSERT INTO orders (id, user_id, total_cents, status, priority, note, created_at) VALUES (?, ?, ?, 'pending', 'low', ?, ?)`,
		id, userID, total, nil, created); err != nil {
		t.Fatalf("order: %v", err)
	}
}

func TestCreateGetListProducts(t *testing.T) {
	s := newTestSetup(t)
	ctx := t.Context()
	insertCategory(t, s, "cat-1", "Gadgets")

	created, err := s.svc.CreateProduct(ctx, &genshop.CreateProductRequest{Name: "Widget", PriceCents: 2500})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Headline != "Widget" || created.PriceCents != 2500 || created.Id == "" {
		t.Fatalf("created = %+v, want Widget/2500 with id", created)
	}

	got, err := s.svc.GetProduct(ctx, &genshop.GetProductRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Id != created.Id || got.Headline != "Widget" {
		t.Fatalf("got = %+v, want created product", got)
	}

	list, err := s.svc.ListProducts(ctx, &genshop.ListProductsRequest{CategoryId: created.CategoryId}, "", 0)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Id != created.Id {
		t.Fatalf("list len = %d, want 1 created item", len(list.Items))
	}
	if list.NextCursor != "" {
		t.Fatalf("cursor = %q, want empty when done", list.NextCursor)
	}

	// Other categories filter everything out.
	empty, err := s.svc.ListProducts(ctx, &genshop.ListProductsRequest{CategoryId: "cat-1"}, "", 0)
	if err != nil {
		t.Fatalf("filtered list: %v", err)
	}
	if len(empty.Items) != 0 {
		t.Fatalf("filtered list len = %d, want 0", len(empty.Items))
	}

	// Missing product surfaces NotFound.
	if _, err := s.svc.GetProduct(ctx, &genshop.GetProductRequest{Id: "missing"}); codeOf(t, err) != apperror.NotFound {
		t.Fatalf("get missing: err %v, want NotFound", err)
	}
}

func TestCreateProductValidation(t *testing.T) {
	s := newTestSetup(t)
	ctx := t.Context()

	if _, err := s.svc.CreateProduct(ctx, &genshop.CreateProductRequest{Name: "  ", PriceCents: 10}); codeOf(t, err) != apperror.InvalidArgument {
		t.Fatalf("blank name: err %v, want InvalidArgument", err)
	}
	if _, err := s.svc.CreateProduct(ctx, &genshop.CreateProductRequest{Name: "Widget", PriceCents: -1}); codeOf(t, err) != apperror.InvalidArgument {
		t.Fatalf("negative price: err %v, want InvalidArgument", err)
	}
}

func TestCreateProductDuplicate(t *testing.T) {
	s := newTestSetup(t)
	ctx := t.Context()

	if _, err := s.svc.CreateProduct(ctx, &genshop.CreateProductRequest{Name: "Widget", PriceCents: 100}); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := s.svc.CreateProduct(ctx, &genshop.CreateProductRequest{Name: "Widget", PriceCents: 100}); codeOf(t, err) != apperror.AlreadyExists {
		t.Fatalf("duplicate: err %v, want AlreadyExists", err)
	}
}

func TestUpdatePatchDeleteProduct(t *testing.T) {
	s := newTestSetup(t)
	ctx := t.Context()

	created, err := s.svc.CreateProduct(ctx, &genshop.CreateProductRequest{Name: "Widget", PriceCents: 100})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := s.svc.UpdateProduct(ctx, &genshop.UpdateProductRequest{Id: created.Id, Name: "Widget v2"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Headline != "Widget v2" {
		t.Fatalf("headline = %q, want v2", updated.Headline)
	}
	if _, updateErr := s.svc.UpdateProduct(ctx, &genshop.UpdateProductRequest{Id: created.Id, Name: " "}); codeOf(t, updateErr) != apperror.InvalidArgument {
		t.Fatalf("blank update: err %v, want InvalidArgument", updateErr)
	}
	if _, updateErr := s.svc.UpdateProduct(ctx, &genshop.UpdateProductRequest{Id: "missing", Name: "x"}); codeOf(t, updateErr) != apperror.NotFound {
		t.Fatalf("update missing: err %v, want NotFound", updateErr)
	}

	patched, err := s.svc.PatchProduct(ctx, &genshop.PatchProductRequest{Id: created.Id, Stock: 7})
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	if patched.Stock != 7 {
		t.Fatalf("stock = %d, want 7", patched.Stock)
	}
	if _, patchErr := s.svc.PatchProduct(ctx, &genshop.PatchProductRequest{Id: created.Id, Stock: -1}); codeOf(t, patchErr) != apperror.InvalidArgument {
		t.Fatalf("negative stock: err %v, want InvalidArgument", patchErr)
	}
	if _, patchErr := s.svc.PatchProduct(ctx, &genshop.PatchProductRequest{Id: "missing", Stock: 1}); codeOf(t, patchErr) != apperror.NotFound {
		t.Fatalf("patch missing: err %v, want NotFound", patchErr)
	}

	deleted, err := s.svc.DeleteProduct(ctx, &genshop.DeleteProductRequest{Id: created.Id})
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if deleted.Id != created.Id {
		t.Fatalf("deleted id = %q, want %q", deleted.Id, created.Id)
	}
	if _, err := s.svc.GetProduct(ctx, &genshop.GetProductRequest{Id: created.Id}); codeOf(t, err) != apperror.NotFound {
		t.Fatalf("get deleted: err %v, want NotFound", err)
	}
	if _, err := s.svc.DeleteProduct(ctx, &genshop.DeleteProductRequest{Id: created.Id}); codeOf(t, err) != apperror.NotFound {
		t.Fatalf("delete missing: err %v, want NotFound", err)
	}
}

func TestCheckout(t *testing.T) {
	s := newTestSetup(t)
	ctx := t.Context()
	insertUser(t, s, "user-1", "shop@example.com")

	receipt, err := s.svc.Checkout(ctx, &genshop.CheckoutRequest{
		Order:    &pb.CreateOrderRequest{UserId: "user-1", Coupon: "SAVE10", TotalCents: 5000},
		GiftNote: "hi",
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}
	if receipt.OrderId == "" || receipt.TotalCents != 5000 {
		t.Fatalf("receipt = %+v, want id + 5000", receipt)
	}

	// Order persisted as pending/low with the gift note.
	got, err := s.svc.GetOrder(ctxFor(t, s, "user-1"), &genshop.GetOrderRequest{Id: receipt.OrderId})
	if err != nil {
		t.Fatalf("get order: %v", err)
	}
	if got.TotalCents != 5000 || got.Status != pb.OrderStatus_ORDER_STATUS_PENDING || got.Priority != pb.Order_PRIORITY_LOW {
		t.Fatalf("order = %+v, want 5000/pending/low", got)
	}
	if got.Note == nil || *got.Note != "hi" {
		t.Fatalf("note = %+v, want hi", got.Note)
	}

	// Confirmation job enqueued.
	msg, err := s.queue.Pop(ctx, "orders.confirm")
	if err != nil {
		t.Fatalf("pop: %v", err)
	}
	if !strings.Contains(string(msg.Payload), receipt.OrderId) {
		t.Fatalf("payload = %q, want order id", string(msg.Payload))
	}

	// Caller identity fills in a blank order user.
	receipt2, err := s.svc.Checkout(ctxFor(t, s, "user-1"), &genshop.CheckoutRequest{
		Order: &pb.CreateOrderRequest{TotalCents: 100},
	})
	if err != nil {
		t.Fatalf("subject checkout: %v", err)
	}
	got2, err := s.svc.GetOrder(ctxFor(t, s, "user-1"), &genshop.GetOrderRequest{Id: receipt2.OrderId})
	if err != nil {
		t.Fatalf("get subject order: %v", err)
	}
	if got2.UserId != "user-1" {
		t.Fatalf("user = %q, want user-1", got2.UserId)
	}

	// Bad input rejected.
	if _, err := s.svc.Checkout(ctx, &genshop.CheckoutRequest{}); codeOf(t, err) != apperror.InvalidArgument {
		t.Fatalf("nil order: err %v, want InvalidArgument", err)
	}
	if _, err := s.svc.Checkout(ctx, &genshop.CheckoutRequest{Order: &pb.CreateOrderRequest{UserId: "user-1", TotalCents: -5}}); codeOf(t, err) != apperror.InvalidArgument {
		t.Fatalf("negative total: err %v, want InvalidArgument", err)
	}
	if _, err := s.svc.Checkout(ctx, &genshop.CheckoutRequest{Order: &pb.CreateOrderRequest{TotalCents: 5}}); codeOf(t, err) != apperror.Unauthenticated {
		t.Fatalf("anonymous checkout: err %v, want Unauthenticated", err)
	}
}

func TestGetOrderAccess(t *testing.T) {
	s := newTestSetup(t)
	insertUser(t, s, "user-1", "one@example.com")
	insertUser(t, s, "user-2", "two@example.com")
	insertOrder(t, s, "order-1", "user-1", 1200, "2026-01-01T00:00:00Z")

	got, err := s.svc.GetOrder(ctxFor(t, s, "user-1"), &genshop.GetOrderRequest{Id: "order-1"})
	if err != nil {
		t.Fatalf("owner get: %v", err)
	}
	if got.Id != "order-1" || got.TotalCents != 1200 {
		t.Fatalf("got = %+v, want order-1/1200", got)
	}

	if _, err := s.svc.GetOrder(ctxFor(t, s, "user-2"), &genshop.GetOrderRequest{Id: "order-1"}); codeOf(t, err) != apperror.PermissionDenied {
		t.Fatalf("non-owner: err %v, want PermissionDenied", err)
	}
	if _, err := s.svc.GetOrder(ctxFor(t, s, "user-1"), &genshop.GetOrderRequest{Id: "missing"}); codeOf(t, err) != apperror.NotFound {
		t.Fatalf("missing: err %v, want NotFound", err)
	}
	if _, err := s.svc.GetOrder(t.Context(), &genshop.GetOrderRequest{Id: "order-1"}); codeOf(t, err) != apperror.Unauthenticated {
		t.Fatalf("anonymous: err %v, want Unauthenticated", err)
	}
}

func TestListOrders(t *testing.T) {
	s := newTestSetup(t)
	insertUser(t, s, "user-1", "one@example.com")
	insertUser(t, s, "user-2", "two@example.com")
	insertOrder(t, s, "order-old", "user-1", 100, "2026-01-01T00:00:01Z")
	insertOrder(t, s, "order-new", "user-1", 200, "2026-01-01T00:00:02Z")
	insertOrder(t, s, "order-other", "user-2", 300, "2026-01-01T00:00:03Z")

	owner := ctxFor(t, s, "user-1")
	first, err := s.svc.ListOrders(owner, &genshop.ListOrdersRequest{UserId: "user-1"}, "", 1)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(first.Items) != 1 || first.Items[0].Id != "order-new" {
		t.Fatalf("first page = %+v, want [order-new]", first.Items)
	}
	if first.NextCursor == "" {
		t.Fatalf("first page cursor empty, want next page")
	}

	second, err := s.svc.ListOrders(owner, &genshop.ListOrdersRequest{UserId: "user-1"}, first.NextCursor, 1)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 1 || second.Items[0].Id != "order-old" {
		t.Fatalf("second page = %+v, want [order-old]", second.Items)
	}
	if second.NextCursor != "" {
		t.Fatalf("second cursor = %q, want empty when done", second.NextCursor)
	}

	// Scoped to the caller: user-2 sees only their own order.
	other, err := s.svc.ListOrders(ctxFor(t, s, "user-2"), &genshop.ListOrdersRequest{}, "", 0)
	if err != nil {
		t.Fatalf("other list: %v", err)
	}
	if len(other.Items) != 1 || other.Items[0].Id != "order-other" {
		t.Fatalf("other list = %+v, want [order-other]", other.Items)
	}

	// Oversized limits cap at the default instead of failing.
	capped, err := s.svc.ListOrders(owner, &genshop.ListOrdersRequest{}, "", 1000)
	if err != nil {
		t.Fatalf("capped list: %v", err)
	}
	if len(capped.Items) != 2 {
		t.Fatalf("capped len = %d, want 2", len(capped.Items))
	}

	// Anonymous callers are rejected.
	if _, err := s.svc.ListOrders(t.Context(), &genshop.ListOrdersRequest{}, "", 0); codeOf(t, err) != apperror.Unauthenticated {
		t.Fatalf("anonymous list: err %v, want Unauthenticated", err)
	}
}
