package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Judge mirrors admin_judges (the Ops-owned judge pool).
type Judge struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Expertise       string `json:"expertise"`
	ExperienceYears int    `json:"experienceYears"`
	Affiliation     string `json:"affiliation"`
	IsVerified      bool   `json:"isVerified"`
}

type judgeService struct{ pool *pgxpool.Pool }

func (s judgeService) list(ctx context.Context) ([]Judge, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, expertise, experience_years, COALESCE(affiliation,''), is_verified
		 FROM admin_judges ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Judge{}
	for rows.Next() {
		var j Judge
		if err := rows.Scan(&j.ID, &j.Name, &j.Expertise, &j.ExperienceYears, &j.Affiliation, &j.IsVerified); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s judgeService) create(ctx context.Context, j Judge) (*Judge, error) {
	if j.Name == "" || j.Expertise == "" {
		return nil, httpx.ErrBadRequest("name and expertise are required")
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_judges (name, expertise, experience_years, affiliation)
		VALUES ($1,$2,$3,NULLIF($4,'')) RETURNING id, is_verified`,
		j.Name, j.Expertise, j.ExperienceYears, j.Affiliation).Scan(&j.ID, &j.IsVerified)
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (s judgeService) update(ctx context.Context, id string, j Judge) (*Judge, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_judges SET name=$2, expertise=$3, experience_years=$4,
			affiliation=NULLIF($5,''), is_verified=$6, updated_at=now()
		WHERE id=$1`, id, j.Name, j.Expertise, j.ExperienceYears, j.Affiliation, j.IsVerified)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("judge not found")
	}
	j.ID = id
	return &j, nil
}

func (s judgeService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_judges WHERE id=$1`, id)
	return err
}

func registerJudges(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := judgeService{pool}
	mux.Handle("GET /admin/ops/judges", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/judges", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var j Judge
		if err := httpx.Decode(r, &j); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), j)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/judges/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var j Judge
		if err := httpx.Decode(r, &j); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), j)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/judges/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}
