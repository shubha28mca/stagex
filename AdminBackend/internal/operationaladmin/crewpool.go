package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// CrewMember is a reusable crew-pool entry (Admin Design §4.1) with its cost.
type CrewMember struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Role     string  `json:"role"`
	Cost     float64 `json:"cost"`
	Contact  string  `json:"contact"`
	IsActive bool    `json:"isActive"`
}

type crewPoolService struct{ pool *pgxpool.Pool }

func (s crewPoolService) list(ctx context.Context) ([]CrewMember, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, role, cost, COALESCE(contact,''), is_active FROM admin_crew ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CrewMember{}
	for rows.Next() {
		var c CrewMember
		if err := rows.Scan(&c.ID, &c.Name, &c.Role, &c.Cost, &c.Contact, &c.IsActive); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s crewPoolService) create(ctx context.Context, c CrewMember) (*CrewMember, error) {
	if c.Name == "" || c.Role == "" {
		return nil, httpx.ErrBadRequest("name and role are required")
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_crew (name, role, cost, contact)
		VALUES ($1,$2,$3,NULLIF($4,'')) RETURNING id, is_active`,
		c.Name, c.Role, c.Cost, c.Contact).Scan(&c.ID, &c.IsActive)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s crewPoolService) update(ctx context.Context, id string, c CrewMember) (*CrewMember, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_crew SET name=$2, role=$3, cost=$4, contact=NULLIF($5,''), is_active=$6, updated_at=now()
		WHERE id=$1`, id, c.Name, c.Role, c.Cost, c.Contact, c.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("crew member not found")
	}
	c.ID = id
	return &c, nil
}

func (s crewPoolService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_crew WHERE id=$1`, id)
	return err
}

func registerCrewPool(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := crewPoolService{pool}
	mux.Handle("GET /admin/ops/crew", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/crew", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c CrewMember
		if err := httpx.Decode(r, &c); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), c)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/crew/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c CrewMember
		if err := httpx.Decode(r, &c); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), c)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/crew/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}
