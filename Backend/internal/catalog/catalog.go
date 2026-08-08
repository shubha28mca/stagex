// Package catalog exposes read-only access to the Ops-owned master data (the
// admin_ tables): event types, taxonomy categories and age bands. The
// participant app consumes these to render filters and taxonomy, but never
// writes to them — writes belong to the separate Admin Console.
package catalog

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// EventType mirrors admin_event_types.
type EventType struct {
	ID              string `json:"id"`
	Code            string `json:"code"`
	Name            string `json:"name"`
	CertificateSeal string `json:"certificateSeal"`
}

// Category mirrors admin_categories (parent may be empty for top level).
type Category struct {
	ID       string `json:"id"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	ParentID string `json:"parentId,omitempty"`
}

// AgeBand mirrors admin_age_bands.
type AgeBand struct {
	ID     string `json:"id"`
	Code   string `json:"code"`
	Label  string `json:"label"`
	MinAge int    `json:"minAge"`
	MaxAge int    `json:"maxAge"`
}

// Service reads master data. It is deliberately tiny — a single struct with the
// pool — because there is no business logic beyond a read.
type Service struct {
	pool *pgxpool.Pool
}

// NewService builds a catalog Service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// EventTypes returns all active event types.
func (s *Service) EventTypes(ctx context.Context) ([]EventType, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, certificate_seal FROM admin_event_types WHERE is_active ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventType{}
	for rows.Next() {
		var e EventType
		if err := rows.Scan(&e.ID, &e.Code, &e.Name, &e.CertificateSeal); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Categories returns the taxonomy tree (flat list with parentId).
func (s *Service) Categories(ctx context.Context) ([]Category, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, name, COALESCE(parent_id::text,'') FROM admin_categories WHERE is_active ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Category{}
	for rows.Next() {
		var c Category
		if err := rows.Scan(&c.ID, &c.Code, &c.Name, &c.ParentID); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// AgeBands returns all active age bands.
func (s *Service) AgeBands(ctx context.Context) ([]AgeBand, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, label, min_age, max_age FROM admin_age_bands WHERE is_active ORDER BY min_age`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AgeBand{}
	for rows.Next() {
		var a AgeBand
		if err := rows.Scan(&a.ID, &a.Code, &a.Label, &a.MinAge, &a.MaxAge); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Controller adapts HTTP to the catalog Service. This package keeps controller
// and routes in the same file because the surface is small and read-only.
type Controller struct{ svc *Service }

// NewController builds a catalog Controller.
func NewController(svc *Service) *Controller { return &Controller{svc: svc} }

// RegisterRoutes wires the public master-data endpoints.
func RegisterRoutes(mux *http.ServeMux, c *Controller) {
	mux.HandleFunc("GET /api/catalog/event-types", c.eventTypes)
	mux.HandleFunc("GET /api/catalog/categories", c.categories)
	mux.HandleFunc("GET /api/catalog/age-bands", c.ageBands)
}

func (c *Controller) eventTypes(w http.ResponseWriter, r *http.Request) {
	data, err := c.svc.EventTypes(r.Context())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, data)
}

func (c *Controller) categories(w http.ResponseWriter, r *http.Request) {
	data, err := c.svc.Categories(r.Context())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, data)
}

func (c *Controller) ageBands(w http.ResponseWriter, r *http.Request) {
	data, err := c.svc.AgeBands(r.Context())
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, data)
}
