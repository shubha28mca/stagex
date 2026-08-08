package registrations

import (
	"net/http"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Controller adapts HTTP to the registrations Service.
type Controller struct {
	svc *Service
}

// NewController builds a registrations Controller.
func NewController(svc *Service) *Controller {
	return &Controller{svc: svc}
}

// Create handles POST /api/registrations.
func (c *Controller) Create(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	var req createRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	reg, err := c.svc.Create(r.Context(), id.AccountID, req)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, reg)
}
