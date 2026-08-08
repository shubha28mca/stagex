package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Hall mirrors admin_halls (venue registry / master data).
type Hall struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	City        string  `json:"city"`
	Capacity    int     `json:"capacity"`
	BaseRate    float64 `json:"baseRate"`
	LeadName    string  `json:"leadName"`
	LeadContact string  `json:"leadContact"`
	IsActive    bool    `json:"isActive"`
}

type hallService struct{ pool *pgxpool.Pool }

func (s hallService) list(ctx context.Context) ([]Hall, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, city, capacity, base_rate, COALESCE(lead_name,''), COALESCE(lead_contact,''), is_active
		 FROM admin_halls ORDER BY city, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Hall{}
	for rows.Next() {
		var h Hall
		if err := rows.Scan(&h.ID, &h.Name, &h.City, &h.Capacity, &h.BaseRate,
			&h.LeadName, &h.LeadContact, &h.IsActive); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

func (s hallService) create(ctx context.Context, h Hall) (*Hall, error) {
	if h.Name == "" || h.City == "" {
		return nil, httpx.ErrBadRequest("name and city are required")
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_halls (name, city, capacity, base_rate, lead_name, lead_contact)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,'')) RETURNING id, is_active`,
		h.Name, h.City, h.Capacity, h.BaseRate, h.LeadName, h.LeadContact).Scan(&h.ID, &h.IsActive)
	if err != nil {
		return nil, err
	}
	return &h, nil
}

func (s hallService) update(ctx context.Context, id string, h Hall) (*Hall, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_halls SET name=$2, city=$3, capacity=$4, base_rate=$5,
			lead_name=NULLIF($6,''), lead_contact=NULLIF($7,''), is_active=$8, updated_at=now()
		WHERE id=$1`, id, h.Name, h.City, h.Capacity, h.BaseRate, h.LeadName, h.LeadContact, h.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("hall not found")
	}
	h.ID = id
	return &h, nil
}

func (s hallService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_halls WHERE id=$1`, id)
	return err
}

func registerHalls(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := hallService{pool}
	mux.Handle("GET /admin/ops/halls", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/halls", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var h Hall
		if err := httpx.Decode(r, &h); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), h)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/halls/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var h Hall
		if err := httpx.Decode(r, &h); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), h)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/halls/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}
