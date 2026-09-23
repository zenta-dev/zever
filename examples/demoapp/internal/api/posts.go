package api

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	gen "github.com/zenta-dev/zever/examples/demoapp/generated/zenorm/orm/gen/app"
	"github.com/zenta-dev/zever/orm"
)

// listPosts returns up to 100 posts, optionally filtered by user_id.
func (s *Server) listPosts(w http.ResponseWriter, req *http.Request) {
	q := orm.From(gen.Posts).OrderBy(gen.PostCols.CreatedAt.Asc()).Limit(100)
	if uid := req.URL.Query().Get("user_id"); uid != "" {
		q = q.Where(gen.PostCols.UserID.Eq(uid))
	}
	if s.I18n != nil {
		if msg, err := s.I18n.Translate(req.Context(), "en", "welcome", nil); err == nil && msg != "" {
			w.Header().Set("X-Message", msg)
		}
	}
	rows, err := q.All(req.Context(), s.DB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"posts": rows})
}

type createPostRequest struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// createPost inserts a post and demos crypto as a best-effort side effect.
func (s *Server) createPost(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	var body createPostRequest
	if err := decodeJSON(req, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if body.Title == "" {
		writeError(w, http.StatusBadRequest, "title required")
		return
	}

	post := &gen.Post{
		ID:        uuid.NewString(),
		UserID:    subject(req),
		Title:     body.Title,
		Body:      body.Body,
		Published: false,
		CreatedAt: time.Now().UTC(),
	}
	if err := orm.InsertInto(gen.Posts).Values(
		orm.Set(gen.PostCols.ID, post.ID),
		orm.Set(gen.PostCols.UserID, post.UserID),
		orm.Set(gen.PostCols.Title, post.Title),
		orm.Set(gen.PostCols.Body, post.Body),
		orm.Set(gen.PostCols.Published, post.Published),
		orm.Set(gen.PostCols.CreatedAt, post.CreatedAt),
	).Exec(ctx, s.DB); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create")
		return
	}

	if s.Crypto != nil {
		if enc, err := s.Crypto.Encrypt(ctx, []byte(post.Title)); err == nil && len(enc) > 0 {
			w.Header().Set("X-Crypto", "encrypted")
		}
	}

	writeJSON(w, http.StatusCreated, post)
}
