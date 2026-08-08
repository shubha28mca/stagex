package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Category mirrors admin_categories (a self-referencing taxonomy tree).
type Category struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	ParentID string `json:"parentId"`
	IsActive bool   `json:"isActive"`
}

type categoryService struct{ pool *pgxpool.Pool }

func (s categoryService) list(ctx context.Context) ([]Category, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, COALESCE(parent_id::text,''), is_active FROM admin_categories ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.ParentID, &c.IsActive); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s categoryService) create(ctx context.Context, c Category) (*Category, error) {
	if c.Code == "" || c.Name == "" {
		return nil, httpx.ErrBadRequest("code and name are required")
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_categories (code, name, parent_id)
		VALUES ($1,$2,NULLIF($3,'')::uuid) RETURNING id, is_active`,
		c.Code, c.Name, c.ParentID).Scan(&c.ID, &c.IsActive)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s categoryService) update(ctx context.Context, id string, c Category) (*Category, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_categories SET name=$2, parent_id=NULLIF($3,'')::uuid, is_active=$4, updated_at=now()
		WHERE id=$1`, id, c.Name, c.ParentID, c.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("category not found")
	}
	c.ID = id
	return &c, nil
}

func (s categoryService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_categories WHERE id=$1`, id)
	return err
}

func registerCategories(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := categoryService{pool}
	mux.Handle("GET /admin/ops/categories", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/categories", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c Category
		if err := httpx.Decode(r, &c); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), c)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/categories/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c Category
		if err := httpx.Decode(r, &c); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), c)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/categories/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}
