package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// EventRow is the platform-wide view of an event (Admin Design §4.4 — the
// "Created by" oversight Event Admins don't have).
type EventRow struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	City        string  `json:"city"`
	Status      string  `json:"status"`
	Fee         float64 `json:"fee"`
	SlotsFilled int     `json:"slotsFilled"`
	SlotsTotal  int     `json:"slotsTotal"`
	CreatedBy   string  `json:"createdBy"`
	Registrations int   `json:"registrations"`
}

// eventUpdate is the unrestricted edit Ops may apply to any event.
type eventUpdate struct {
	Name   string  `json:"name"`
	City   string  `json:"city"`
	Status string  `json:"status"`
	Fee    float64 `json:"fee"`
}

type oversightService struct{ pool *pgxpool.Pool }

func (s oversightService) listEvents(ctx context.Context) ([]EventRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.name, e.city, e.status, e.fee, e.slots_filled, e.slots_total,
		       COALESCE(au.name,'—'),
		       (SELECT COUNT(*) FROM registrations r WHERE r.event_id = e.id)
		FROM events e
		LEFT JOIN admin_users au ON au.id = e.created_by
		ORDER BY e.start_date DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventRow{}
	for rows.Next() {
		var e EventRow
		if err := rows.Scan(&e.ID, &e.Name, &e.City, &e.Status, &e.Fee, &e.SlotsFilled,
			&e.SlotsTotal, &e.CreatedBy, &e.Registrations); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s oversightService) updateEvent(ctx context.Context, id string, u eventUpdate) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE events SET name=$2, city=$3, status=$4, fee=$5, updated_at=now() WHERE id=$1`,
		id, u.Name, u.City, u.Status, u.Fee)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrNotFound("event not found")
	}
	return nil
}

// deleteEvent force-removes an event and every dependent record in one
// transaction — Ops delete is unrestricted per the requirement.
func (s oversightService) deleteEvent(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	stmts := []string{
		`DELETE FROM entries WHERE registration_id IN (SELECT id FROM registrations WHERE event_id=$1)`,
		`DELETE FROM payments WHERE registration_id IN (SELECT id FROM registrations WHERE event_id=$1)`,
		`DELETE FROM registrations WHERE event_id=$1`,
		`DELETE FROM certificates WHERE event_id=$1`,
		`DELETE FROM feedback WHERE event_id=$1`,
		`DELETE FROM event_categories WHERE event_id=$1`,
		`DELETE FROM events WHERE id=$1`,
	}
	for _, q := range stmts {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// Dashboard is the Ops summary (Admin Design §4.10, condensed).
type Dashboard struct {
	Events        int     `json:"events"`
	Registrations int     `json:"registrations"`
	Participants  int     `json:"participants"`
	Revenue       float64 `json:"revenue"`
}

func (s oversightService) dashboard(ctx context.Context) (Dashboard, error) {
	var d Dashboard
	err := s.pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM events),
		       (SELECT COUNT(*) FROM registrations),
		       (SELECT COUNT(*) FROM people WHERE deleted_at IS NULL),
		       (SELECT COALESCE(SUM(total),0) FROM registrations WHERE status='paid')`).
		Scan(&d.Events, &d.Registrations, &d.Participants, &d.Revenue)
	return d, err
}

func registerOversight(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := oversightService{pool}
	mux.Handle("GET /admin/ops/dashboard", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.dashboard(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("GET /admin/ops/events", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.listEvents(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("PATCH /admin/ops/events/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u eventUpdate
		if err := httpx.Decode(r, &u); err != nil {
			httpx.Error(w, err)
			return
		}
		respond(w, map[string]bool{"updated": true}, svc.updateEvent(r.Context(), r.PathValue("id"), u))
	})))
	mux.Handle("DELETE /admin/ops/events/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.deleteEvent(r.Context(), r.PathValue("id")))
	})))
}
