// Package eventadmin contains everything the Event Admin owns: creating,
// publishing, editing and deleting their own events, managing the categories
// inside an event, and viewing/editing the participants registered for their
// events. It is a self-contained folder kept separate from operationaladmin,
// though both share one database. Every route is gated to the 'event' role and
// scoped so an Event Admin only ever touches events they created.
package eventadmin

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Event is an Event Admin's own event.
type Event struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Tagline       string    `json:"tagline"`
	City          string    `json:"city"`
	Mode          string    `json:"mode"`
	Rounds        int       `json:"rounds"`
	Fee           float64   `json:"fee"`
	SlotsTotal    int       `json:"slotsTotal"`
	SlotsFilled   int       `json:"slotsFilled"`
	StartDate     time.Time `json:"startDate"`
	EndDate       time.Time `json:"endDate"`
	Status        string    `json:"status"`
	CoverGradient string    `json:"coverGradient"`
	Registrations int       `json:"registrations"`
	RoundsDetail  []Round     `json:"roundsDetail"`
	Rubric        []Criterion `json:"rubric"`
	JudgeIDs      []string    `json:"judgeIds"`
}

// Round is one named round (wizard step: Rounds).
type Round struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Criterion is one judging-rubric line with a weight percentage.
type Criterion struct {
	Criterion string `json:"criterion"`
	Weight    int    `json:"weight"`
}

type createEventRequest struct {
	Name          string      `json:"name"`
	Tagline       string      `json:"tagline"`
	City          string      `json:"city"`
	Mode          string      `json:"mode"`
	Rounds        int         `json:"rounds"`
	Fee           float64     `json:"fee"`
	SlotsTotal    int         `json:"slotsTotal"`
	StartDate     string      `json:"startDate"`
	EndDate       string      `json:"endDate"`
	CoverGradient string      `json:"coverGradient"`
	RoundsDetail  []Round     `json:"roundsDetail"`
	Rubric        []Criterion `json:"rubric"`
	JudgeIDs      []string    `json:"judgeIds"`
}

// jsonbArray marshals a slice for a jsonb column, using '[]' for empty/nil so
// the column never holds SQL null (keeps reads simple).
func jsonbArray(v any) string {
	b, err := json.Marshal(v)
	if err != nil || len(b) == 0 || string(b) == "null" {
		return "[]"
	}
	return string(b)
}

type eventService struct{ pool *pgxpool.Pool }

func (s eventService) listMine(ctx context.Context, adminID string) ([]Event, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.id, e.name, e.tagline, e.city, e.mode, e.rounds, e.fee, e.slots_total,
		       e.slots_filled, e.start_date, e.end_date, e.status, e.cover_gradient,
		       COALESCE(e.rounds_detail,'[]'), COALESCE(e.rubric,'[]'), COALESCE(e.judge_ids,'[]'),
		       (SELECT COUNT(*) FROM registrations r WHERE r.event_id = e.id)
		FROM events e WHERE e.created_by = $1 ORDER BY e.start_date DESC`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var roundsJSON, rubricJSON, judgesJSON []byte
		if err := rows.Scan(&e.ID, &e.Name, &e.Tagline, &e.City, &e.Mode, &e.Rounds, &e.Fee,
			&e.SlotsTotal, &e.SlotsFilled, &e.StartDate, &e.EndDate, &e.Status,
			&e.CoverGradient, &roundsJSON, &rubricJSON, &judgesJSON, &e.Registrations); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(roundsJSON, &e.RoundsDetail)
		_ = json.Unmarshal(rubricJSON, &e.Rubric)
		_ = json.Unmarshal(judgesJSON, &e.JudgeIDs)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s eventService) create(ctx context.Context, adminID string, req createEventRequest) (*Event, error) {
	if req.Name == "" || req.City == "" {
		return nil, httpx.ErrBadRequest("name and city are required")
	}
	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return nil, httpx.ErrBadRequest("startDate must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return nil, httpx.ErrBadRequest("endDate must be YYYY-MM-DD")
	}
	if end.Before(start) {
		return nil, httpx.ErrBadRequest("endDate cannot be before startDate")
	}
	if req.Mode == "" {
		req.Mode = "onstage"
	}
	if req.Rounds < 1 {
		req.Rounds = 1
	}
	if req.CoverGradient == "" {
		req.CoverGradient = "purple"
	}
	var e Event
	err = s.pool.QueryRow(ctx, `
		INSERT INTO events (name, tagline, city, mode, rounds, fee, slots_total,
		                    start_date, end_date, status, cover_gradient, created_by,
		                    rounds_detail, rubric, judge_ids)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'draft',$10,$11,$12::jsonb,$13::jsonb,$14::jsonb)
		RETURNING id, name, tagline, city, mode, rounds, fee, slots_total, slots_filled,
		          start_date, end_date, status, cover_gradient`,
		req.Name, req.Tagline, req.City, req.Mode, req.Rounds, req.Fee, req.SlotsTotal,
		start, end, req.CoverGradient, adminID,
		jsonbArray(req.RoundsDetail), jsonbArray(req.Rubric), jsonbArray(req.JudgeIDs)).
		Scan(&e.ID, &e.Name, &e.Tagline, &e.City, &e.Mode, &e.Rounds, &e.Fee, &e.SlotsTotal,
			&e.SlotsFilled, &e.StartDate, &e.EndDate, &e.Status, &e.CoverGradient)
	if err != nil {
		return nil, err
	}
	e.RoundsDetail, e.Rubric, e.JudgeIDs = req.RoundsDetail, req.Rubric, req.JudgeIDs
	return &e, nil
}

func (s eventService) update(ctx context.Context, adminID, id string, req createEventRequest) error {
	start, err := time.Parse("2006-01-02", req.StartDate)
	if err != nil {
		return httpx.ErrBadRequest("startDate must be YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", req.EndDate)
	if err != nil {
		return httpx.ErrBadRequest("endDate must be YYYY-MM-DD")
	}
	ct, err := s.pool.Exec(ctx, `
		UPDATE events SET name=$3, tagline=$4, city=$5, mode=$6, rounds=$7, fee=$8,
			slots_total=$9, start_date=$10, end_date=$11, cover_gradient=$12,
			rounds_detail=$13::jsonb, rubric=$14::jsonb, judge_ids=$15::jsonb, updated_at=now()
		WHERE id=$1 AND created_by=$2`,
		id, adminID, req.Name, req.Tagline, req.City, req.Mode, req.Rounds, req.Fee,
		req.SlotsTotal, start, end, req.CoverGradient,
		jsonbArray(req.RoundsDetail), jsonbArray(req.Rubric), jsonbArray(req.JudgeIDs))
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrNotFound("event not found or not yours")
	}
	return nil
}

func (s eventService) setStatus(ctx context.Context, adminID, id, status string) error {
	ct, err := s.pool.Exec(ctx,
		`UPDATE events SET status=$3, updated_at=now() WHERE id=$1 AND created_by=$2`,
		id, adminID, status)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrNotFound("event not found or not yours")
	}
	return nil
}

func (s eventService) del(ctx context.Context, adminID, id string) error {
	// Confirm ownership first, then cascade-delete dependents.
	var owner string
	if err := s.pool.QueryRow(ctx, `SELECT COALESCE(created_by::text,'') FROM events WHERE id=$1`, id).Scan(&owner); err != nil {
		return httpx.ErrNotFound("event not found")
	}
	if owner != adminID {
		return httpx.ErrForbidden("you can only delete events you created")
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, q := range []string{
		`DELETE FROM entries WHERE registration_id IN (SELECT id FROM registrations WHERE event_id=$1)`,
		`DELETE FROM payments WHERE registration_id IN (SELECT id FROM registrations WHERE event_id=$1)`,
		`DELETE FROM registrations WHERE event_id=$1`,
		`DELETE FROM certificates WHERE event_id=$1`,
		`DELETE FROM feedback WHERE event_id=$1`,
		`DELETE FROM event_categories WHERE event_id=$1`,
		`DELETE FROM events WHERE id=$1`,
	} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func registerEvents(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := eventService{pool}
	mux.Handle("GET /admin/event/events", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.listMine(r.Context(), id.AdminID)
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/event/events", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var req createEventRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), id.AdminID, req)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/event/events/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var req createEventRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, err)
			return
		}
		respond(w, map[string]bool{"updated": true}, svc.update(r.Context(), id.AdminID, r.PathValue("id"), req))
	})))
	mux.Handle("POST /admin/event/events/{id}/publish", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		respond(w, map[string]bool{"published": true}, svc.setStatus(r.Context(), id.AdminID, r.PathValue("id"), "open"))
	})))
	mux.Handle("DELETE /admin/event/events/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		respond(w, map[string]bool{"deleted": true}, svc.del(r.Context(), id.AdminID, r.PathValue("id")))
	})))
}

// respond / respondCreated are shared helpers for this package's controllers.
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

// eventOwnedBy reports whether the event was created by this admin — the guard
// every per-event feature uses so an Event Admin only touches their own events.
func eventOwnedBy(ctx context.Context, pool *pgxpool.Pool, adminID, eventID string) (bool, error) {
	var n int
	err := pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE id=$1 AND created_by=$2`, eventID, adminID).Scan(&n)
	return n > 0, err
}
