// Package certificates lists every certificate earned across the family, with
// the person, event, position and a downloadable file URL (ClientDesignWeb §8).
package certificates

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Certificate is one award earned by a person.
type Certificate struct {
	ID           string    `json:"id"`
	PersonName   string    `json:"personName"`
	EventName    string    `json:"eventName"`
	CategoryName string    `json:"categoryName"`
	Position     string    `json:"position"`
	CertCode     string    `json:"certCode"`
	FileURL      string    `json:"fileUrl,omitempty"`
	IssuedAt     time.Time `json:"issuedAt"`
}

// Service reads the family's certificates.
type Service struct{ pool *pgxpool.Pool }

// NewService builds a certificates Service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// List returns every certificate for people under the family.
func (s *Service) List(ctx context.Context, familyID string) ([]Certificate, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT ct.id, p.name, e.name, ct.category_name, ct.position, ct.cert_code,
		       COALESCE(ct.file_url,''), ct.issued_at
		FROM certificates ct
		JOIN people p ON p.id = ct.person_id
		JOIN events e ON e.id = ct.event_id
		WHERE p.family_id = $1
		ORDER BY ct.issued_at DESC`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Certificate{}
	for rows.Next() {
		var c Certificate
		if err := rows.Scan(&c.ID, &c.PersonName, &c.EventName, &c.CategoryName,
			&c.Position, &c.CertCode, &c.FileURL, &c.IssuedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Controller adapts HTTP to the certificates Service.
type Controller struct{ svc *Service }

// NewController builds a certificates Controller.
func NewController(svc *Service) *Controller { return &Controller{svc: svc} }

// RegisterRoutes wires the protected certificates endpoint.
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/my/certificates", protect(http.HandlerFunc(c.list)))
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
