// Package shop implements the ShopService business logic declared in
// schema/shop/shop.zen over the generated gogen wire types and the
// zenorm typed tables.
package shop

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zenta-dev/zever/apperror"
	"github.com/zenta-dev/zever/authz"
	"github.com/zenta-dev/zever/db"
	"github.com/zenta-dev/zever/log"
	"github.com/zenta-dev/zever/orm"
	"github.com/zenta-dev/zever/queue"

	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
	pb "github.com/zenta-dev/zever/examples/showcase/generated/protogogen/shop"
	shoporm "github.com/zenta-dev/zever/examples/showcase/generated/zenorm/orm/gen/shop"
)

// defaultPageSize is the page size used when the caller passes no limit,
// and the cap applied to any larger limit.
const defaultPageSize = 50

// fallbackCategoryID is the category assigned to products created through
// CreateProduct. The CreateProduct RPC carries only a name and a price, so
// it cannot name a category; the impl keeps the products.category_id
// foreign key valid by ensuring this fallback row exists instead of
// inventing per-call categories.
const fallbackCategoryID = "uncategorized"

// confirmTopic is the queue topic a successful Checkout publishes to,
// mirroring the internal/api checkout handler.
const confirmTopic = "orders.confirm"

// Deps carries the infrastructure a ShopServiceImpl needs.
type Deps struct {
	// DB is the database the service reads and writes.
	DB db.DB
	// Queue receives order-confirmation jobs; nil disables publishing.
	Queue queue.Queue
	// Logger is kept for future structured logging; currently unused.
	Logger log.Logger
}

// ShopServiceImpl implements genshop.ShopService.
//
//nolint:revive // name matches the generated service-stub convention (NewShopServiceImpl) so entrypoint wiring stays regenerate-compatible.
type ShopServiceImpl struct {
	db    db.DB
	queue queue.Queue
}

// NewShopServiceImpl builds a ShopServiceImpl from deps.
func NewShopServiceImpl(d Deps) *ShopServiceImpl {
	return &ShopServiceImpl{db: d.DB, queue: d.Queue}
}

var _ genshop.ShopService = (*ShopServiceImpl)(nil)

// ListProducts returns products newest first, optionally filtered by
// category, paginated with an opaque offset cursor.
func (s *ShopServiceImpl) ListProducts(ctx context.Context, req *genshop.ListProductsRequest, cursor string, limit int32) (*genshop.ListProductsResponse, error) {
	offset, err := parseCursor(cursor)
	if err != nil {
		return nil, err
	}
	page := pageLimit(limit)

	q := orm.From(shoporm.Products)
	if req != nil && req.CategoryId != "" {
		q = q.Where(shoporm.ProductCols.CategoryID.Eq(req.CategoryId))
	}
	rows, err := q.OrderBy(shoporm.ProductCols.CreatedAt.Desc()).Offset(offset).Limit(page+1).All(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "list products failed", err)
	}

	resp := &genshop.ListProductsResponse{}
	for _, r := range rows {
		if len(resp.Items) == page {
			resp.NextCursor = strconv.Itoa(offset + page)
			break
		}
		resp.Items = append(resp.Items, toProtoProduct(r))
	}
	return resp, nil
}

// GetProduct returns one product by id, or NotFound.
func (s *ShopServiceImpl) GetProduct(ctx context.Context, req *genshop.GetProductRequest) (*genshop.Product, error) {
	if req == nil {
		return nil, apperror.New(apperror.InvalidArgument, "request required")
	}
	p, ok, err := orm.From(shoporm.Products).Where(shoporm.ProductCols.ID.Eq(req.Id)).First(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "lookup failed", err)
	}
	if !ok {
		return nil, apperror.New(apperror.NotFound, "product not found")
	}
	return toProtoProduct(p), nil
}

// CreateProduct inserts a product. The RPC carries only name and price, so
// the name doubles as headline and sku (keeping sku unique per name) and
// the row lands in the fallback category.
func (s *ShopServiceImpl) CreateProduct(ctx context.Context, req *genshop.CreateProductRequest) (*genshop.Product, error) {
	if req == nil {
		return nil, apperror.New(apperror.InvalidArgument, "request required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperror.New(apperror.InvalidArgument, "name required")
	}
	if req.PriceCents < 0 {
		return nil, apperror.New(apperror.InvalidArgument, "negative price")
	}

	if err := s.ensureFallbackCategory(ctx); err != nil {
		return nil, err
	}

	id := uuid.NewString()
	now := time.Now().UTC()
	err := orm.InsertInto(shoporm.Products).Values(
		orm.Set(shoporm.ProductCols.ID, id),
		orm.Set(shoporm.ProductCols.CategoryID, fallbackCategoryID),
		orm.Set(shoporm.ProductCols.Sku, name),
		orm.Set(shoporm.ProductCols.Headline, name),
		orm.Set(shoporm.ProductCols.Description, ""),
		orm.Set(shoporm.ProductCols.PriceCents, req.PriceCents),
		orm.Set(shoporm.ProductCols.Stock, int64(0)),
		orm.Set(shoporm.ProductCols.Weight, float64(0)),
		orm.Set(shoporm.ProductCols.Featured, false),
		orm.Set(shoporm.ProductCols.CreatedAt, now),
	).Exec(ctx, s.db)
	if err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			return nil, apperror.New(apperror.AlreadyExists, "product sku taken")
		}
		return nil, apperror.Wrap(apperror.Internal, "create failed", err)
	}

	p, ok, err := orm.From(shoporm.Products).Where(shoporm.ProductCols.ID.Eq(id)).First(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "lookup failed", err)
	}
	if !ok {
		return nil, apperror.New(apperror.Internal, "product vanished after create")
	}
	return toProtoProduct(p), nil
}

// UpdateProduct replaces a product's headline, or NotFound.
func (s *ShopServiceImpl) UpdateProduct(ctx context.Context, req *genshop.UpdateProductRequest) (*genshop.Product, error) {
	if req == nil {
		return nil, apperror.New(apperror.InvalidArgument, "request required")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, apperror.New(apperror.InvalidArgument, "headline required")
	}

	n, err := orm.UpdateTable(shoporm.Products).
		Where(shoporm.ProductCols.ID.Eq(req.Id)).
		Set(orm.Set(shoporm.ProductCols.Headline, name)).
		Exec(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "update failed", err)
	}
	if n == 0 {
		return nil, apperror.New(apperror.NotFound, "product not found")
	}
	return s.GetProduct(ctx, &genshop.GetProductRequest{Id: req.Id})
}

// PatchProduct sets a product's stock, or NotFound. Negative stock is
// InvalidArgument.
func (s *ShopServiceImpl) PatchProduct(ctx context.Context, req *genshop.PatchProductRequest) (*genshop.Product, error) {
	if req == nil {
		return nil, apperror.New(apperror.InvalidArgument, "request required")
	}
	if req.Stock < 0 {
		return nil, apperror.New(apperror.InvalidArgument, "negative stock")
	}

	n, err := orm.UpdateTable(shoporm.Products).
		Where(shoporm.ProductCols.ID.Eq(req.Id)).
		Set(orm.Set(shoporm.ProductCols.Stock, req.Stock)).
		Exec(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "update failed", err)
	}
	if n == 0 {
		return nil, apperror.New(apperror.NotFound, "product not found")
	}
	return s.GetProduct(ctx, &genshop.GetProductRequest{Id: req.Id})
}

// DeleteProduct removes a product and returns it, or NotFound when missing.
// Permission is enforced by the generated wrappers, not here.
func (s *ShopServiceImpl) DeleteProduct(ctx context.Context, req *genshop.DeleteProductRequest) (*genshop.Product, error) {
	if req == nil {
		return nil, apperror.New(apperror.InvalidArgument, "request required")
	}
	p, ok, err := orm.From(shoporm.Products).Where(shoporm.ProductCols.ID.Eq(req.Id)).First(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "lookup failed", err)
	}
	if !ok {
		return nil, apperror.New(apperror.NotFound, "product not found")
	}
	if _, err := orm.DeleteFrom(shoporm.Products).Where(shoporm.ProductCols.ID.Eq(req.Id)).Exec(ctx, s.db); err != nil {
		return nil, apperror.Wrap(apperror.Internal, "delete failed", err)
	}
	return toProtoProduct(p), nil
}

// Checkout creates a pending low-priority order for the user named in
// req.Order (falling back to the caller's identity) and enqueues a
// confirmation job. The wire CheckoutRequest carries no line items, so no
// items or stock movement happen here.
func (s *ShopServiceImpl) Checkout(ctx context.Context, req *genshop.CheckoutRequest) (*genshop.OrderReceipt, error) {
	if req == nil || req.Order == nil {
		return nil, apperror.New(apperror.InvalidArgument, "order required")
	}
	if len(req.GiftNote) > 500 {
		return nil, apperror.New(apperror.InvalidArgument, "gift_note too long")
	}
	if req.Order.TotalCents < 0 {
		return nil, apperror.New(apperror.InvalidArgument, "negative total")
	}

	userID := req.Order.UserId
	if userID == "" {
		sub, ok := callerSubject(ctx)
		if !ok {
			return nil, apperror.New(apperror.Unauthenticated, "unauthenticated")
		}
		userID = sub
	}

	orderID := uuid.NewString()
	note := strings.TrimSpace(req.GiftNote)
	noteAssign := shoporm.OrderCols.Note.SetNull()
	if note != "" {
		noteAssign = shoporm.OrderCols.Note.SetValue(note)
	}
	err := orm.InsertInto(shoporm.Orders).Values(
		orm.Set(shoporm.OrderCols.ID, orderID),
		orm.Set(shoporm.OrderCols.UserID, userID),
		orm.Set(shoporm.OrderCols.TotalCents, req.Order.TotalCents),
		orm.Set(shoporm.OrderCols.Status, shoporm.OrderStatusPending),
		orm.Set(shoporm.OrderCols.Priority, "low"),
		noteAssign,
		orm.Set(shoporm.OrderCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "create failed", err)
	}

	if s.queue != nil {
		body, _ := json.Marshal(map[string]string{"order_id": orderID})
		_ = s.queue.Push(ctx, confirmTopic, queue.NewPayload(body), queue.NewHeaders(nil))
	}

	return &genshop.OrderReceipt{OrderId: orderID, TotalCents: req.Order.TotalCents}, nil
}

// GetOrder returns one order owned by the caller: missing rows are NotFound,
// rows owned by someone else are PermissionDenied.
func (s *ShopServiceImpl) GetOrder(ctx context.Context, req *genshop.GetOrderRequest) (*genshop.Order, error) {
	if req == nil {
		return nil, apperror.New(apperror.InvalidArgument, "request required")
	}
	o, ok, err := orm.From(shoporm.Orders).Where(shoporm.OrderCols.ID.Eq(req.Id)).First(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "lookup failed", err)
	}
	if !ok {
		return nil, apperror.New(apperror.NotFound, "order not found")
	}
	sub, ok := callerSubject(ctx)
	if !ok {
		return nil, apperror.New(apperror.Unauthenticated, "unauthenticated")
	}
	if o.UserID != sub {
		return nil, apperror.New(apperror.PermissionDenied, "no access")
	}
	return toProtoOrder(o), nil
}

// ListOrders returns the caller's orders newest first, paginated like
// ListProducts. The scope always comes from the caller identity, never
// from the request body.
func (s *ShopServiceImpl) ListOrders(ctx context.Context, _ *genshop.ListOrdersRequest, cursor string, limit int32) (*genshop.ListOrdersResponse, error) {
	sub, ok := callerSubject(ctx)
	if !ok {
		return nil, apperror.New(apperror.Unauthenticated, "unauthenticated")
	}
	offset, err := parseCursor(cursor)
	if err != nil {
		return nil, err
	}
	page := pageLimit(limit)

	rows, err := orm.From(shoporm.Orders).
		Where(shoporm.OrderCols.UserID.Eq(sub)).
		OrderBy(shoporm.OrderCols.CreatedAt.Desc()).
		Offset(offset).Limit(page+1).
		All(ctx, s.db)
	if err != nil {
		return nil, apperror.Wrap(apperror.Internal, "list orders failed", err)
	}

	resp := &genshop.ListOrdersResponse{}
	for _, r := range rows {
		if len(resp.Items) == page {
			resp.NextCursor = strconv.Itoa(offset + page)
			break
		}
		resp.Items = append(resp.Items, toProtoOrder(r))
	}
	return resp, nil
}

// subjectKey is the context key ContextWithSubject writes and callerSubject
// reads. It exists because the hand-written /api adapters authenticate with
// their own JWT check and cannot mint authz claims (unexported key).
type subjectKey struct{}

// ContextWithSubject returns a context carrying sub as the caller identity
// for the shop service. This is the single write-once home usable from both
// the generated wrappers (which also set authz claims) and hand-written
// adapters: callerSubject reads this key first, then falls back to
// authz.ClaimsFromContext.
func ContextWithSubject(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, subjectKey{}, sub)
}

// callerSubject returns the authenticated caller's subject, or false when
// the context carries no identity.
func callerSubject(ctx context.Context) (string, bool) {
	if sub, ok := ctx.Value(subjectKey{}).(string); ok && sub != "" {
		return sub, true
	}
	claims, ok := authz.ClaimsFromContext(ctx)
	if !ok || claims.Subject == "" {
		return "", false
	}
	return claims.Subject, true
}

// pageLimit normalizes a requested page size: non-positive means the
// default, anything above the default is capped.
func pageLimit(limit int32) int {
	if limit <= 0 {
		return defaultPageSize
	}
	if int(limit) > defaultPageSize {
		return defaultPageSize
	}
	return int(limit)
}

// parseCursor decodes an offset cursor; empty means the first page.
func parseCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(cursor)
	if err != nil || n < 0 {
		return 0, apperror.New(apperror.InvalidArgument, "invalid cursor")
	}
	return n, nil
}

// ensureFallbackCategory inserts the fallback category when absent so
// product inserts never violate the category foreign key.
func (s *ShopServiceImpl) ensureFallbackCategory(ctx context.Context) error {
	_, ok, err := orm.From(shoporm.Categories).Where(shoporm.CategoryCols.ID.Eq(fallbackCategoryID)).First(ctx, s.db)
	if err != nil {
		return apperror.Wrap(apperror.Internal, "lookup failed", err)
	}
	if ok {
		return nil
	}
	err = orm.InsertInto(shoporm.Categories).Values(
		orm.Set(shoporm.CategoryCols.ID, fallbackCategoryID),
		orm.Set(shoporm.CategoryCols.Name, fallbackCategoryID),
		orm.Set(shoporm.CategoryCols.Description, "Default category for products created without an explicit category"),
		orm.Set(shoporm.CategoryCols.CreatedAt, time.Now().UTC()),
	).Exec(ctx, s.db)
	if err != nil {
		return apperror.Wrap(apperror.Internal, "create failed", err)
	}
	return nil
}

// toProtoProduct maps a zenorm product row onto the wire product.
func toProtoProduct(p *shoporm.Product) *genshop.Product {
	return &genshop.Product{
		Id:          p.ID,
		CategoryId:  p.CategoryID,
		Sku:         p.Sku,
		Headline:    p.Headline,
		Description: p.Description,
		PriceCents:  p.PriceCents,
		Stock:       p.Stock,
		Weight:      p.Weight,
		Featured:    p.Featured,
		CreatedAt:   timestamppb.New(p.CreatedAt),
	}
}

// toProtoOrder maps a zenorm order row onto the wire order.
func toProtoOrder(o *shoporm.Order) *genshop.Order {
	out := &genshop.Order{
		Id:         o.ID,
		UserId:     o.UserID,
		TotalCents: o.TotalCents,
		Status:     orderStatusToProto(o.Status),
		Priority:   orderPriorityToProto(o.Priority),
		CreatedAt:  timestamppb.New(o.CreatedAt),
	}
	if note, ok := o.Note.Get(); ok {
		out.Note = &note
	}
	return out
}

// orderStatusToProto maps stored status text onto the wire enum.
func orderStatusToProto(s shoporm.OrderStatus) pb.OrderStatus {
	switch s {
	case shoporm.OrderStatusPending:
		return pb.OrderStatus_ORDER_STATUS_PENDING
	case shoporm.OrderStatusPaid:
		return pb.OrderStatus_ORDER_STATUS_PAID
	case shoporm.OrderStatusShipped:
		return pb.OrderStatus_ORDER_STATUS_SHIPPED
	case shoporm.OrderStatusCancelled:
		return pb.OrderStatus_ORDER_STATUS_CANCELLED
	default:
		return pb.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

// orderPriorityToProto maps stored priority text onto the wire enum.
func orderPriorityToProto(p string) pb.Order_PriorityEnum {
	switch p {
	case "low":
		return pb.Order_PRIORITY_LOW
	case "high":
		return pb.Order_PRIORITY_HIGH
	default:
		return pb.Order_PRIORITY_UNSPECIFIED
	}
}
