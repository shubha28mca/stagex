package events

import (
	"net/http"
	"strconv"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Controller adapts HTTP requests to the events Service.
type Controller struct {
	svc *Service
}

// NewController builds an events Controller.
func NewController(svc *Service) *Controller {
	return &Controller{svc: svc}
}

// List handles GET /events with optional query-string filters.
func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := Filter{
		Query:  q.Get("query"),
		City:   q.Get("city"),
		Mode:   q.Get("mode"),
		Status: q.Get("status"),
	}
	if v := q.Get("maxFee"); v != "" {
		f.MaxFee, _ = strconv.ParseFloat(v, 64)
	}
	if v := q.Get("rounds"); v != "" {
		f.Rounds, _ = strconv.Atoi(v)
	}
	list, err := c.svc.List(r.Context(), f)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

// Get handles GET /events/{id}.
func (c *Controller) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	e, err := c.svc.Get(r.Context(), id)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, e)
}
