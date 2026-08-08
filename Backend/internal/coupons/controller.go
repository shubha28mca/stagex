package coupons

import (
	"net/http"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Controller adapts HTTP to the coupons Service.
type Controller struct {
	svc *Service
}

// NewController builds a coupons Controller.
func NewController(svc *Service) *Controller {
	return &Controller{svc: svc}
}

// validateRequest is the body for POST /coupons/validate.
type validateRequest struct {
	Code     string  `json:"code"`
	Subtotal float64 `json:"subtotal"`
	EventID  string  `json:"eventId"`
}

// Validate handles POST /api/coupons/validate.
func (c *Controller) Validate(w http.ResponseWriter, r *http.Request) {
	var req validateRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	res, err := c.svc.Validate(r.Context(), req.Code, req.Subtotal, req.EventID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
