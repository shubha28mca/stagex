package people

import (
	"net/http"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Controller adapts HTTP to the people Service. Every handler resolves the
// caller's family from the authenticated identity — people are never addressed
// across families.
type Controller struct {
	svc *Service
}

// NewController builds a people Controller.
func NewController(svc *Service) *Controller {
	return &Controller{svc: svc}
}

// List handles GET /api/people.
func (c *Controller) List(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	list, err := c.svc.List(r.Context(), id.AccountID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, list)
}

// Create handles POST /api/people.
func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	var req createRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	p, err := c.svc.Create(r.Context(), id.AccountID, req)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, p)
}

// Update handles PATCH /api/people/{id}.
func (c *Controller) Update(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	var req updateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	p, err := c.svc.Update(r.Context(), r.PathValue("id"), id.AccountID, req)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, p)
}

// Delete handles DELETE /api/people/{id}. Depending on the person's event
// attachments the result is either a full removal or a retained soft delete.
func (c *Controller) Delete(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	res, err := c.svc.Delete(r.Context(), r.PathValue("id"), id.AccountID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
