package payments

import (
	"net/http"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Controller adapts HTTP to the payments Service.
type Controller struct {
	svc *Service
}

// NewController builds a payments Controller.
func NewController(svc *Service) *Controller {
	return &Controller{svc: svc}
}

// CreateOrder handles POST /api/payments/order.
func (c *Controller) CreateOrder(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	var req createOrderRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	order, err := c.svc.CreateOrder(r.Context(), id.AccountID, req.RegistrationID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, order)
}

// Confirm handles POST /api/payments/confirm (stands in for the Razorpay webhook).
func (c *Controller) Confirm(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	var req confirmRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	res, err := c.svc.Confirm(r.Context(), id.AccountID, req.RegistrationID, req.Success)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}
