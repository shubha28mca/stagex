package eventadmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// EventCategory is a category slot inside an event (age-banded).
type EventCategory struct {
	ID                string  `json:"id"`
	CategoryID        string  `json:"categoryId"`
	CategoryName      string  `json:"categoryName"`
	AgeBandID         string  `json:"ageBandId"`
	AgeBandLabel      string  `json:"ageBandLabel"`
	ParticipationType string  `json:"participationType"`
	Fee               float64 `json:"fee"`
}

type addCategoryRequest struct {
	CategoryID        string  `json:"categoryId"`
	AgeBandID         string  `json:"ageBandId"`
	ParticipationType string  `json:"participationType"`
	Fee               float64 `json:"fee"`
}

// refItem is a lightweight {id,label} option for the category builder.
type refItem struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type categoryService struct{ pool *pgxpool.Pool }

// ownsEvent verifies the event belongs to the admin before any mutation.
func (s categoryService) ownsEvent(ctx context.Context, adminID, eventID string) (bool, error) {
	var n int
	err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM events WHERE id=$1 AND created_by=$2`, eventID, adminID).Scan(&n)
	return n > 0, err
}

func (s categoryService) list(ctx context.Context, adminID, eventID string) ([]EventCategory, error) {
	ok, err := s.ownsEvent(ctx, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ec.id, c.id, c.name, ab.id, ab.label, ec.participation_type, ec.fee
		FROM event_categories ec
		JOIN admin_categories c ON c.id = ec.category_id
		JOIN admin_age_bands ab ON ab.id = ec.age_band_id
		WHERE ec.event_id = $1 ORDER BY c.name`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventCategory{}
	for rows.Next() {
		var e EventCategory
		if err := rows.Scan(&e.ID, &e.CategoryID, &e.CategoryName, &e.AgeBandID,
			&e.AgeBandLabel, &e.ParticipationType, &e.Fee); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s categoryService) add(ctx context.Context, adminID, eventID string, req addCategoryRequest) error {
	ok, err := s.ownsEvent(ctx, adminID, eventID)
	if err != nil {
		return err
	}
	if !ok {
		return httpx.ErrNotFound("event not found or not yours")
	}
	if req.CategoryID == "" || req.AgeBandID == "" {
		return httpx.ErrBadRequest("categoryId and ageBandId are required")
	}
	if req.ParticipationType == "" {
		req.ParticipationType = "solo"
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO event_categories (event_id, category_id, age_band_id, participation_type, fee)
		VALUES ($1,$2,$3,$4,$5)`,
		eventID, req.CategoryID, req.AgeBandID, req.ParticipationType, req.Fee)
	return err
}

// remove deletes an event category the admin owns (verified through the join).
func (s categoryService) remove(ctx context.Context, adminID, ecID string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM event_categories ec
		USING events e
		WHERE ec.id=$1 AND ec.event_id = e.id AND e.created_by=$2`, ecID, adminID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrNotFound("category not found or not yours")
	}
	return nil
}

func (s categoryService) refCategories(ctx context.Context) ([]refItem, error) {
	return s.refList(ctx, `SELECT id, name FROM admin_categories WHERE is_active ORDER BY name`)
}
func (s categoryService) refAgeBands(ctx context.Context) ([]refItem, error) {
	return s.refList(ctx, `SELECT id, label FROM admin_age_bands WHERE is_active ORDER BY min_age`)
}
func (s categoryService) refJudges(ctx context.Context) ([]refItem, error) {
	return s.refList(ctx, `SELECT id, name || ' — ' || expertise FROM admin_judges WHERE is_verified ORDER BY name`)
}
func (s categoryService) refList(ctx context.Context, q string) ([]refItem, error) {
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []refItem{}
	for rows.Next() {
		var it refItem
		if err := rows.Scan(&it.ID, &it.Label); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func registerCategories(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := categoryService{pool}
	mux.Handle("GET /admin/event/events/{id}/categories", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.list(r.Context(), id.AdminID, r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/event/events/{id}/categories", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var req addCategoryRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, err)
			return
		}
		respondCreated(w, map[string]bool{"added": true}, svc.add(r.Context(), id.AdminID, r.PathValue("id"), req))
	})))
	mux.Handle("DELETE /admin/event/categories/{ecId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		respond(w, map[string]bool{"deleted": true}, svc.remove(r.Context(), id.AdminID, r.PathValue("ecId")))
	})))
	mux.Handle("GET /admin/event/ref/categories", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.refCategories(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("GET /admin/event/ref/age-bands", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.refAgeBands(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("GET /admin/event/ref/judges", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.refJudges(r.Context())
		respond(w, data, err)
	})))
}
