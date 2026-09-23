package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/permission"

	genshop "github.com/zenta-dev/zever/examples/showcase/generated/gogen/shop"
)

type productRequest struct {
	CategoryID  string  `json:"category_id"`
	SKU         string  `json:"sku"`
	Headline    string  `json:"headline"`
	Description string  `json:"description"`
	PriceCents  int64   `json:"price_cents"`
	Stock       int64   `json:"stock"`
	Weight      float64 `json:"weight"`
	Featured    bool    `json:"featured"`
}

type productResponse struct {
	ID          string  `json:"id"`
	CategoryID  string  `json:"category_id"`
	SKU         string  `json:"sku"`
	Headline    string  `json:"headline"`
	Description string  `json:"description"`
	PriceCents  int64   `json:"price_cents"`
	Stock       int64   `json:"stock"`
	Weight      float64 `json:"weight"`
	Featured    bool    `json:"featured"`
	CreatedAt   string  `json:"created_at"`
}

// handleCreateProduct inserts a product. Admin role required, mirroring the
// schema's auth: required(roles: {admin}).
func (a *API) handleCreateProduct(w http.ResponseWriter, req *http.Request) {
	var in productRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	in.SKU = strings.TrimSpace(in.SKU)
	in.Headline = strings.TrimSpace(in.Headline)
	if in.SKU == "" || in.Headline == "" {
		writeError(w, http.StatusBadRequest, "sku and headline required")
		return
	}
	if in.PriceCents < 0 {
		writeError(w, http.StatusBadRequest, "negative price")
		return
	}

	if !a.isAdmin(req) {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}

	ctx := req.Context()
	rows, err := a.DB.Query(ctx, `SELECT id FROM categories WHERE id = ?`, in.CategoryID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	hasCat := rows.Next()
	_ = rows.Close()
	if !hasCat {
		writeError(w, http.StatusBadRequest, "unknown category")
		return
	}

	id := uuid.NewString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	featured := 0
	if in.Featured {
		featured = 1
	}
	if _, err := a.DB.Exec(ctx,
		`INSERT INTO products (id, category_id, sku, headline, description, price_cents, stock, weight, featured, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, in.CategoryID, in.SKU, in.Headline, in.Description, in.PriceCents, in.Stock, in.Weight, featured, now,
	); err != nil {
		if strings.Contains(strings.ToUpper(err.Error()), "UNIQUE") {
			writeError(w, http.StatusConflict, "product sku taken")
			return
		}
		writeError(w, http.StatusInternalServerError, "create failed")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"id": id, "message": a.productMessage(ctx, req, in.Headline, in.PriceCents)})
}

// handleListProducts returns products, rate-limited per caller. Thin
// adapter over the shared impl; the JSON shape is unchanged.
func (a *API) handleListProducts(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	if a.Ratelimit != nil {
		dec, err := a.Ratelimit.Allow(ctx, "products:list:"+subject(req), 1)
		if err != nil || !dec.Allowed {
			writeError(w, http.StatusTooManyRequests, "rate limited")
			return
		}
	}

	resp, err := a.shopService().ListProducts(shopReq(req).Context(), &genshop.ListProductsRequest{}, "", 0) //nolint:contextcheck // subject deliberately derived into the impl context.
	if err != nil {
		writeServiceError(w, err)
		return
	}
	out := []productResponse{}
	for _, p := range resp.Items {
		out = append(out, productToResponse(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetProduct returns one product. Thin adapter over the shared impl.
// Public in the schema (auth: none); the route stays behind withAuth here
// because this server scopes every /api route to authenticated callers.
func (a *API) handleGetProduct(w http.ResponseWriter, req *http.Request) {
	p, err := a.shopService().GetProduct(shopReq(req).Context(), &genshop.GetProductRequest{Id: req.PathValue("id")}) //nolint:contextcheck // subject deliberately derived into the impl context.
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, productToResponse(p))
}

type updateProductRequest struct {
	Headline    string `json:"headline"`
	Description string `json:"description"`
}

// handleUpdateProduct replaces headline/description (PUT semantics).
func (a *API) handleUpdateProduct(w http.ResponseWriter, req *http.Request) {
	if !a.isAdmin(req) {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	var in updateProductRequest
	if err := decodeJSON(req, &in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(in.Headline) == "" {
		writeError(w, http.StatusBadRequest, "headline required")
		return
	}
	ctx := req.Context()
	id := req.PathValue("id")
	n, err := a.DB.Exec(ctx, `UPDATE products SET headline = ?, description = ? WHERE id = ?`, strings.TrimSpace(in.Headline), in.Description, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

type patchProductRequest struct {
	Stock *int64 `json:"stock"`
}

// handlePatchProduct adjusts stock only (PATCH semantics). Thin adapter:
// admin gate and input validation stay here, the stock write lives in the
// shared impl.
func (a *API) handlePatchProduct(w http.ResponseWriter, req *http.Request) {
	if !a.isAdmin(req) {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	var in patchProductRequest
	if err := decodeJSON(req, &in); err != nil || in.Stock == nil {
		writeError(w, http.StatusBadRequest, "stock required")
		return
	}
	id := req.PathValue("id")
	if _, err := a.shopService().PatchProduct(shopReq(req).Context(), &genshop.PatchProductRequest{Id: id, Stock: *in.Stock}); err != nil { //nolint:contextcheck // subject deliberately derived into the impl context.
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

// handleDeleteProduct removes a product after the rbac permission check,
// mirroring the schema's permission: check("product.delete", ...).
func (a *API) handleDeleteProduct(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	id := req.PathValue("id")

	rows, err := a.DB.Query(ctx, `SELECT category_id FROM products WHERE id = ?`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "lookup failed")
		return
	}
	var categoryID string
	if rows.Next() {
		if err := rows.Scan(&categoryID); err != nil {
			_ = rows.Close()
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
	} else {
		_ = rows.Close()
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	_ = rows.Close()

	if a.Permission != nil {
		dec, err := a.Permission.Can(ctx,
			permission.Subject{ID: subject(req), Roles: []string{a.roleOf(req)}},
			"product.delete",
			permission.Resource{Type: "Product", ID: id, Attributes: map[string]string{"owner": categoryID}})
		if err != nil || !dec.Allowed {
			writeError(w, http.StatusForbidden, "no access")
			return
		}
	}

	if _, err := a.shopService().DeleteProduct(shopReq(req).Context(), &genshop.DeleteProductRequest{Id: id}); err != nil { //nolint:contextcheck // subject deliberately derived into the impl context.
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"id": id})
}

// productToResponse maps a wire product onto the /api JSON shape.
func productToResponse(p *genshop.Product) productResponse {
	out := productResponse{
		ID:          p.GetId(),
		CategoryID:  p.GetCategoryId(),
		SKU:         p.GetSku(),
		Headline:    p.GetHeadline(),
		Description: p.GetDescription(),
		PriceCents:  p.GetPriceCents(),
		Stock:       p.GetStock(),
		Weight:      p.GetWeight(),
		Featured:    p.GetFeatured(),
	}
	if ts := p.GetCreatedAt(); ts != nil {
		out.CreatedAt = ts.AsTime().UTC().Format(time.RFC3339Nano)
	}
	return out
}

// roleOf returns the caller's users.role, or "" when unknown.
func (a *API) roleOf(req *http.Request) string {
	ctx := req.Context()
	rows, err := a.DB.Query(ctx, `SELECT role FROM users WHERE id = ?`, subject(req))
	if err != nil {
		return ""
	}
	var role string
	if rows.Next() {
		if err := rows.Scan(&role); err != nil {
			_ = rows.Close()
			return ""
		}
	}
	_ = rows.Close()
	return role
}

// isAdmin reports whether the caller is an admin: the JWT subject's
// users.role must be 'admin'. Role elevation happens out of band (seed
// data or direct update), never through registration.
func (a *API) isAdmin(req *http.Request) bool {
	return a.roleOf(req) == "admin"
}

// productMessage renders the product-created string via i18n with fallback.
func (a *API) productMessage(ctx context.Context, req *http.Request, name string, price int64) string {
	if a.I18n == nil {
		return "Product listed: " + name
	}
	msg, err := a.I18n.Translate(ctx, locale(req), "product.created",
		map[string]string{"Name": name, "Price": strconv.FormatInt(price, 10)})
	if err != nil || msg == "" {
		return "Product listed: " + name
	}
	return msg
}
