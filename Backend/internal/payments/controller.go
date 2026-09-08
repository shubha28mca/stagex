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

// Verify handles POST /api/payments/verify (Razorpay signature verification / test confirmation).
func (c *Controller) Verify(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	var req VerifyRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	res, err := c.svc.VerifyPayment(r.Context(), id.AccountID, req)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, res)
}

// Confirm handles POST /api/payments/confirm (backward-compatibility).
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
