// Package feedback records one rating submission per family per event, only
// after the event has completed (ClientDesignWeb §9). Ratings cover overall,
// judges, venue, food stalls, schedule and sponsor booths.
package feedback

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Ratings are the 1–5 star answers. Each is validated to the 1..5 range.
type Ratings struct {
	Overall  int `json:"overall"`
	Judges   int `json:"judges"`
	Venue    int `json:"venue"`
	Food     int `json:"food"`
	Schedule int `json:"schedule"`
	Sponsors int `json:"sponsors"`
}

type submitRequest struct {
	EventID   string  `json:"eventId"`
	Ratings   Ratings `json:"ratings"`
	Comment   string  `json:"comment"`
	Anonymous *bool   `json:"anonymous"`
}

// Service validates and stores feedback.
type Service struct{ pool *pgxpool.Pool }

// NewService builds a feedback Service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// Submit stores one feedback row, enforcing: the event exists and is completed,
// the ratings are in range, and one-submission-per-family-per-event.
func (s *Service) Submit(ctx context.Context, familyID string, req submitRequest) error {
	if req.EventID == "" {
		return httpx.ErrBadRequest("eventId is required")
	}
	for _, v := range []int{req.Ratings.Overall, req.Ratings.Judges, req.Ratings.Venue,
		req.Ratings.Food, req.Ratings.Schedule, req.Ratings.Sponsors} {
		if v < 1 || v > 5 {
			return httpx.ErrBadRequest("each rating must be between 1 and 5 stars")
		}
	}

	var status string
	err := s.pool.QueryRow(ctx, `SELECT status FROM events WHERE id=$1`, req.EventID).Scan(&status)
	if err == pgx.ErrNoRows {
		return httpx.ErrNotFound("event not found")
	}
	if err != nil {
		return err
	}
	if status != "completed" {
		return httpx.ErrBadRequest("feedback is available only after the event completes")
	}

	anon := true
	if req.Anonymous != nil {
		anon = *req.Anonymous
	}
	ratingsJSON, _ := json.Marshal(req.Ratings)

	_, err = s.pool.Exec(ctx, `
		INSERT INTO feedback (family_id, event_id, ratings, comment, anonymous)
		VALUES ($1,$2,$3,NULLIF($4,''),$5)
		ON CONFLICT (family_id, event_id)
		DO UPDATE SET ratings=EXCLUDED.ratings, comment=EXCLUDED.comment, anonymous=EXCLUDED.anonymous`,
		familyID, req.EventID, ratingsJSON, req.Comment, anon)
	return err
}

// Controller adapts HTTP to the feedback Service.
type Controller struct{ svc *Service }

// NewController builds a feedback Controller.
func NewController(svc *Service) *Controller { return &Controller{svc: svc} }

// RegisterRoutes wires the protected feedback endpoint.
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	mux.Handle("POST /api/feedback", protect(http.HandlerFunc(c.submit)))
}

func (c *Controller) submit(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	var req submitRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := c.svc.Submit(r.Context(), id.AccountID, req); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]bool{"submitted": true})
}
