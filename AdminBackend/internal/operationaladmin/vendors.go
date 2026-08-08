package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Vendor is a reusable vendor-pool entry (Admin Design §4.3).
type Vendor struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	ServiceType string `json:"serviceType"`
	City        string `json:"city"`
	Contact     string `json:"contact"`
	IsActive    bool   `json:"isActive"`
}

type vendorService struct{ pool *pgxpool.Pool }

func (s vendorService) list(ctx context.Context) ([]Vendor, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, service_type, COALESCE(city,''), COALESCE(contact,''), is_active FROM admin_vendors ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Vendor{}
	for rows.Next() {
		var v Vendor
		if err := rows.Scan(&v.ID, &v.Name, &v.ServiceType, &v.City, &v.Contact, &v.IsActive); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s vendorService) create(ctx context.Context, v Vendor) (*Vendor, error) {
	if v.Name == "" || v.ServiceType == "" {
		return nil, httpx.ErrBadRequest("name and service type are required")
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_vendors (name, service_type, city, contact)
		VALUES ($1,$2,NULLIF($3,''),NULLIF($4,'')) RETURNING id, is_active`,
		v.Name, v.ServiceType, v.City, v.Contact).Scan(&v.ID, &v.IsActive)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (s vendorService) update(ctx context.Context, id string, v Vendor) (*Vendor, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_vendors SET name=$2, service_type=$3, city=NULLIF($4,''), contact=NULLIF($5,''), is_active=$6, updated_at=now()
		WHERE id=$1`, id, v.Name, v.ServiceType, v.City, v.Contact, v.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("vendor not found")
	}
	v.ID = id
	return &v, nil
}

func (s vendorService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_vendors WHERE id=$1`, id)
	return err
}

func registerVendors(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := vendorService{pool}
	mux.Handle("GET /admin/ops/vendors", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/vendors", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v Vendor
		if err := httpx.Decode(r, &v); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), v)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/vendors/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var v Vendor
		if err := httpx.Decode(r, &v); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), v)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/vendors/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}
