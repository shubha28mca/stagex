package auth

import (
	"net/http"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Controller adapts HTTP to the auth Service.
type Controller struct {
	svc *Service
}

// NewController builds an auth Controller.
func NewController(svc *Service) *Controller {
	return &Controller{svc: svc}
}

// SendOTP handles POST /api/auth/otp/send.
func (c *Controller) SendOTP(w http.ResponseWriter, r *http.Request) {
	var req sendOTPRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	resp, err := c.svc.SendOTP(r.Context(), req.Phone, req.Purpose)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}

// VerifyOTP handles POST /api/auth/otp/verify.
func (c *Controller) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req verifyOTPRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	if err := c.svc.VerifyOTP(r.Context(), req.Phone, req.Code, req.Purpose); err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]bool{"verified": true})
}

// Register handles POST /api/auth/register.
func (c *Controller) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	resp, err := c.svc.Register(r.Context(), req.Phone, req.Name, req.Password, req.OTP)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, resp)
}

// Login handles POST /api/auth/login.
func (c *Controller) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.Error(w, err)
		return
	}
	resp, err := c.svc.Login(r.Context(), req.Phone, req.Password, req.OTP)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, resp)
}
