package eventadmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Crew is a person assigned to work an event (Admin Design §4.1).
type Crew struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	Contact string `json:"contact"`
}

type crewService struct{ pool *pgxpool.Pool }

func (s crewService) list(ctx context.Context, adminID, eventID string) ([]Crew, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, role, COALESCE(contact,'') FROM admin_event_crew WHERE event_id=$1 ORDER BY role, name`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Crew{}
	for rows.Next() {
		var c Crew
		if err := rows.Scan(&c.ID, &c.Name, &c.Role, &c.Contact); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s crewService) add(ctx context.Context, adminID, eventID string, c Crew) (*Crew, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	if c.Name == "" || c.Role == "" {
		return nil, httpx.ErrBadRequest("name and role are required")
	}
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_event_crew (event_id, name, role, contact)
		VALUES ($1,$2,$3,NULLIF($4,'')) RETURNING id`,
		eventID, c.Name, c.Role, c.Contact).Scan(&c.ID); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s crewService) remove(ctx context.Context, adminID, crewID string) error {
	// The join guarantees the admin owns the event this crew row belongs to.
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM admin_event_crew c USING events e
		WHERE c.id=$1 AND c.event_id=e.id AND e.created_by=$2`, crewID, adminID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrNotFound("crew member not found or not yours")
	}
	return nil
}

func registerCrew(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := crewService{pool}
	mux.Handle("GET /admin/event/events/{id}/crew", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.list(r.Context(), id.AdminID, r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/event/events/{id}/crew", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var c Crew
		if err := httpx.Decode(r, &c); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.add(r.Context(), id.AdminID, r.PathValue("id"), c)
		respondCreated(w, out, err)
	})))
	mux.Handle("DELETE /admin/event/crew/{crewId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		respond(w, map[string]bool{"deleted": true}, svc.remove(r.Context(), id.AdminID, r.PathValue("crewId")))
	})))
}
