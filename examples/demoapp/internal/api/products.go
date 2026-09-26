package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/zenta-dev/zever/core/permission"
	"github.com/zenta-dev/zever/core/router"
	"github.com/zenta-dev/zever/core/search"
	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

// listProducts returns up to 100 products, optionally filtered by
// category_id. It also exercises the flag and permission batteries.
func (s *Server) listProducts(w http.ResponseWriter, req *http.Request) {
	if s.Flag != nil {
		if on, _ := s.Flag.Bool(req.Context(), "new_checkout", false); on {
			w.Header().Set("X-Flag-new_checkout", "true")
		}
	}
	if s.Permission != nil {
		_, _ = s.Permission.Can(req.Context(),
			permission.Subject{ID: "anon", Roles: []string{"member"}},
			"product.read", permission.Resource{Type: "product", ID: "*"})
	}

	q := orm.From(gen.Products).OrderBy(gen.ProductCols.CreatedAt.Asc()).Limit(100)
	if cat := req.URL.Query().Get("category_id"); cat != "" {
		q = q.Where(gen.ProductCols.CategoryID.Eq(cat))
	}
	rows, err := q.All(req.Context(), s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"products": rows})
}

// getProduct returns one product and caches its name as a demo side effect.
func (s *Server) getProduct(w http.ResponseWriter, req *http.Request) {
	id := router.Param(req, "id")
	row, found, err := orm.From(gen.Products).Where(gen.ProductCols.ID.Eq(id)).First(req.Context(), s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	if s.Cache != nil {
		_ = s.Cache.Set(req.Context(), "product:"+id, []byte(row.Name), time.Minute)
	}
	writeJSON(w, http.StatusOK, row)
}

type createProductRequest struct {
	Name        string `json:"name"`
	CategoryID  string `json:"category_id"`
	Description string `json:"description"`
	PriceCents  int64  `json:"price_cents"`
	Stock       int64  `json:"stock"`
}

// createProduct inserts a product. It exercises ratelimit, idempotency,
// eventbus, and search as best-effort side effects.
func (s *Server) createProduct(w http.ResponseWriter, req *http.Request) {
	var body createProductRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}

	ctx := req.Context()
	if s.RateLimit != nil {
		decision, err := s.RateLimit.Allow(ctx, "create_product", 1)
		if err == nil && !decision.Allowed {
			writeError(w, http.StatusTooManyRequests, "rate limited")
			return
		}
	}

	categoryID := body.CategoryID
	if categoryID == "" {
		first, found, err := orm.From(gen.Categories).Limit(1).First(ctx, s.DB)
		if err != nil || !found {
			writeError(w, http.StatusBadRequest, "category_id required")
			return
		}
		categoryID = first.ID
	}

	p := &gen.Product{
		ID:          uuid.NewString(),
		CategoryID:  categoryID,
		Name:        body.Name,
		Description: body.Description,
		PriceCents:  body.PriceCents,
		Stock:       body.Stock,
		CreatedAt:   time.Now().UTC(),
	}
	if err := orm.InsertInto(gen.Products).Values(
		orm.Set(gen.ProductCols.ID, p.ID),
		orm.Set(gen.ProductCols.CategoryID, p.CategoryID),
		orm.Set(gen.ProductCols.Name, p.Name),
		orm.Set(gen.ProductCols.Description, p.Description),
		orm.Set(gen.ProductCols.PriceCents, p.PriceCents),
		orm.Set(gen.ProductCols.Stock, p.Stock),
		orm.Set(gen.ProductCols.CreatedAt, p.CreatedAt),
	).Exec(ctx, s.DB); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create")
		return
	}

	if s.EventBus != nil {
		_ = s.EventBus.Publish(ctx, "product.created", []byte(p.ID), nil)
	}
	if s.Search != nil {
		_ = s.Search.Index(ctx, search.Document{ID: p.ID, Content: p.Name + " " + p.Description})
	}

	writeJSON(w, http.StatusCreated, p)
}
