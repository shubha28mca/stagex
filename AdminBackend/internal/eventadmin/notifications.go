package eventadmin

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Notification is a broadcast the Event Admin sent; participants read these.
type Notification struct {
	ID        string `json:"id"`
	Audience  string `json:"audience"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

type sendNotificationRequest struct {
	Audience string `json:"audience"` // all | paid | pending
	Title    string `json:"title"`
	Message  string `json:"message"`
}

type notifService struct{ pool *pgxpool.Pool }

// getConfig returns the per-event trigger/channel config (arbitrary JSON the
// frontend defines), defaulting to an empty object.
func (s notifService) getConfig(ctx context.Context, adminID, eventID string) (json.RawMessage, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	var cfg []byte
	err = s.pool.QueryRow(ctx, `SELECT config FROM admin_notification_config WHERE event_id=$1`, eventID).Scan(&cfg)
	if err == pgx.ErrNoRows {
		return json.RawMessage(`{}`), nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(cfg), nil
}

func (s notifService) setConfig(ctx context.Context, adminID, eventID string, cfg json.RawMessage) error {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return err
	}
	if !owns {
		return httpx.ErrNotFound("event not found or not yours")
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO admin_notification_config (event_id, config) VALUES ($1,$2::jsonb)
		ON CONFLICT (event_id) DO UPDATE SET config=EXCLUDED.config, updated_at=now()`,
		eventID, string(cfg))
	return err
}

func (s notifService) list(ctx context.Context, adminID, eventID string) ([]Notification, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, audience, title, message, to_char(created_at,'YYYY-MM-DD HH24:MI')
		FROM admin_notifications WHERE event_id=$1 ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.Audience, &n.Title, &n.Message, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (s notifService) send(ctx context.Context, adminID, eventID string, req sendNotificationRequest) error {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return err
	}
	if !owns {
		return httpx.ErrNotFound("event not found or not yours")
	}
	if req.Title == "" || req.Message == "" {
		return httpx.ErrBadRequest("title and message are required")
	}
	if req.Audience == "" {
		req.Audience = "all"
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO admin_notifications (event_id, audience, title, message)
		VALUES ($1,$2,$3,$4)`, eventID, req.Audience, req.Title, req.Message)
	return err
}

func registerNotifications(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := notifService{pool}
	mux.Handle("GET /admin/event/events/{id}/notifications/config", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.getConfig(r.Context(), id.AdminID, r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("PUT /admin/event/events/{id}/notifications/config", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var cfg json.RawMessage
		if err := httpx.Decode(r, &cfg); err != nil {
			httpx.Error(w, err)
			return
		}
		respond(w, map[string]bool{"saved": true}, svc.setConfig(r.Context(), id.AdminID, r.PathValue("id"), cfg))
	})))
	mux.Handle("GET /admin/event/events/{id}/notifications", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.list(r.Context(), id.AdminID, r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/event/events/{id}/notifications", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var req sendNotificationRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, err)
			return
		}
		respondCreated(w, map[string]bool{"sent": true}, svc.send(r.Context(), id.AdminID, r.PathValue("id"), req))
	})))
}
