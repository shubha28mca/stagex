// Package operationaladmin contains everything the Operational Admin owns:
// master-data CRUD (event types, taxonomy, age bands, coupons, halls, judges,
// sponsors), platform-wide event oversight and unrestricted participant edits.
// It is a self-contained folder — its own repositories, services and
// controllers — kept separate from the eventadmin package though both share the
// one database. Every route in this package is gated to the 'ops' role.
package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// EventType mirrors admin_event_types.
type EventType struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	CertificateSeal string `json:"certificateSeal"`
	Description     string `json:"description"`
	IsActive        bool   `json:"isActive"`
}

type eventTypeService struct{ pool *pgxpool.Pool }

func (s eventTypeService) list(ctx context.Context) ([]EventType, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, certificate_seal, COALESCE(description,''), is_active
		 FROM admin_event_types ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventType{}
	for rows.Next() {
		var e EventType
		if err := rows.Scan(&e.ID, &e.Code, &e.Name, &e.CertificateSeal, &e.Description, &e.IsActive); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s eventTypeService) create(ctx context.Context, e EventType) (*EventType, error) {
	if e.Code == "" || e.Name == "" {
		return nil, httpx.ErrBadRequest("code and name are required")
	}
	if e.CertificateSeal == "" {
		e.CertificateSeal = "standard"
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_event_types (code, name, certificate_seal, description)
		VALUES ($1,$2,$3,$4) RETURNING id, is_active`,
		e.Code, e.Name, e.CertificateSeal, e.Description).Scan(&e.ID, &e.IsActive)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s eventTypeService) update(ctx context.Context, id string, e EventType) (*EventType, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_event_types
		SET name=$2, certificate_seal=$3, description=$4, is_active=$5, updated_at=now()
		WHERE id=$1`, id, e.Name, e.CertificateSeal, e.Description, e.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("event type not found")
	}
	e.ID = id
	return &e, nil
}

func (s eventTypeService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_event_types WHERE id=$1`, id)
	return err
}

func registerEventTypes(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := eventTypeService{pool}
	mux.Handle("GET /admin/ops/event-types", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/event-types", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e EventType
		if err := httpx.Decode(r, &e); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), e)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/event-types/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e EventType
		if err := httpx.Decode(r, &e); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), e)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/event-types/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}

// respond / respondCreated are tiny helpers shared across this package's
// controllers to keep the CRUD handlers to a few lines each.
func respond(w http.ResponseWriter, data any, err error) {
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, data)
}

func respondCreated(w http.ResponseWriter, data any, err error) {
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, data)
}
