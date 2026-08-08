package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// AgeBand mirrors admin_age_bands.
type AgeBand struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Label    string `json:"label"`
	MinAge   int    `json:"minAge"`
	MaxAge   int    `json:"maxAge"`
	IsActive bool   `json:"isActive"`
}

type ageBandService struct{ pool *pgxpool.Pool }

func (s ageBandService) list(ctx context.Context) ([]AgeBand, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, label, min_age, max_age, is_active FROM admin_age_bands ORDER BY min_age`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgeBand{}
	for rows.Next() {
		var a AgeBand
		if err := rows.Scan(&a.ID, &a.Code, &a.Label, &a.MinAge, &a.MaxAge, &a.IsActive); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s ageBandService) create(ctx context.Context, a AgeBand) (*AgeBand, error) {
	if a.Code == "" || a.Label == "" {
		return nil, httpx.ErrBadRequest("code and label are required")
	}
	if a.MaxAge < a.MinAge {
		return nil, httpx.ErrBadRequest("max age must be >= min age")
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_age_bands (code, label, min_age, max_age)
		VALUES ($1,$2,$3,$4) RETURNING id, is_active`,
		a.Code, a.Label, a.MinAge, a.MaxAge).Scan(&a.ID, &a.IsActive)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s ageBandService) update(ctx context.Context, id string, a AgeBand) (*AgeBand, error) {
	if a.MaxAge < a.MinAge {
		return nil, httpx.ErrBadRequest("max age must be >= min age")
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_age_bands SET label=$2, min_age=$3, max_age=$4, is_active=$5, updated_at=now()
		WHERE id=$1`, id, a.Label, a.MinAge, a.MaxAge, a.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("age band not found")
	}
	a.ID = id
	return &a, nil
}

func (s ageBandService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_age_bands WHERE id=$1`, id)
	return err
}

func registerAgeBands(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := ageBandService{pool}
	mux.Handle("GET /admin/ops/age-bands", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/age-bands", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var a AgeBand
		if err := httpx.Decode(r, &a); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), a)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/age-bands/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var a AgeBand
		if err := httpx.Decode(r, &a); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), a)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/age-bands/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}
