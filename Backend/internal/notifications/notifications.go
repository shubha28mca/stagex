// Package notifications lets a participant read the broadcasts an Event Admin
// sent for events the family is registered in (admin_notifications). This is the
// read side of the Admin Console's ad-hoc broadcast composer.
package notifications

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Notification is one broadcast message shown to the participant.
type Notification struct {
	EventName string `json:"eventName"`
	Title     string `json:"title"`
	Message   string `json:"message"`
	CreatedAt string `json:"createdAt"`
}

// Service reads broadcasts targeted at the family's events.
type Service struct{ pool *pgxpool.Pool }

// NewService builds a notifications Service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// List returns broadcasts for events the family is registered in, honoring the
// audience filter (all / paid / pending).
func (s *Service) List(ctx context.Context, familyID string) ([]Notification, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT e.name, n.title, n.message, to_char(n.created_at,'YYYY-MM-DD HH24:MI')
		FROM admin_notifications n
		JOIN events e ON e.id = n.event_id
		WHERE n.event_id IN (SELECT event_id FROM registrations WHERE family_id=$1)
		  AND (
		    n.audience = 'all'
		    OR (n.audience = 'paid'    AND EXISTS (SELECT 1 FROM registrations r WHERE r.family_id=$1 AND r.event_id=n.event_id AND r.status='paid'))
		    OR (n.audience = 'pending' AND EXISTS (SELECT 1 FROM registrations r WHERE r.family_id=$1 AND r.event_id=n.event_id AND r.status<>'paid'))
		  )
		ORDER BY n.created_at DESC`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Notification{}
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.EventName, &n.Title, &n.Message, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// Controller adapts HTTP to the notifications Service.
type Controller struct{ svc *Service }

// NewController builds a notifications Controller.
func NewController(svc *Service) *Controller { return &Controller{svc: svc} }

// RegisterRoutes wires the protected notifications endpoint.
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/my/notifications", protect(http.HandlerFunc(c.list)))
}

func (c *Controller) list(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	data, err := c.svc.List(r.Context(), id.AccountID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, data)
}
