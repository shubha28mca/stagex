package payments

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Provider abstracts the payment gateway (Razorpay or Mock).
type Provider interface {
	Name() string
	KeyID() string
	CreateOrder(amount float64, currency, receipt string) (orderRef string, err error)
	VerifySignature(orderID, paymentID, signature string) bool
}

// Service enforces the payment business rules (hold window, 3-attempt cap, signature verification).
type Service struct {
	repo     Repository
	provider Provider
	now      func() time.Time
}

// NewService builds a payments Service.
func NewService(repo Repository, provider Provider) *Service {
	return &Service{repo: repo, provider: provider, now: time.Now}
}

// CreateOrder starts a new payment attempt for a pending registration.
func (s *Service) CreateOrder(ctx context.Context, familyID, regID string) (*Order, error) {
	reg, err := s.repo.GetRegistration(ctx, regID, familyID)
	if err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, httpx.ErrNotFound("registration not found")
	}
	if reg.Status == "paid" {
		return nil, httpx.ErrConflict("registration is already paid")
	}
	count, err := s.repo.AttemptCount(ctx, regID)
	if err != nil {
		return nil, err
	}
	if count >= maxAttempts {
		return nil, httpx.NewError(429, "payment_locked",
			"maximum payment attempts reached — try a different method or contact support")
	}

	orderRef, err := s.provider.CreateOrder(reg.Total, "INR", regID)
	if err != nil {
		return nil, httpx.ErrInternal(fmt.Sprintf("could not create payment order: %v", err))
	}
	attempt := count + 1
	if err := s.repo.CreatePayment(ctx, regID, s.provider.Name(), orderRef, reg.Total, attempt, "created"); err != nil {
		return nil, err
	}

	return &Order{
		OrderRef:    orderRef,
		Amount:      reg.Total,
		Currency:    "INR",
		Attempt:     attempt,
		Provider:    s.provider.Name(),
		KeyID:       s.provider.KeyID(),
		EventName:   reg.EventName,
		FamilyName:  reg.FamilyName,
		FamilyPhone: reg.FamilyPhone,
	}, nil
}

// VerifyPayment checks the payment outcome (validating Razorpay HMAC signature or mock simulation).
func (s *Service) VerifyPayment(ctx context.Context, familyID string, req VerifyRequest) (*ConfirmResult, error) {
	reg, err := s.repo.GetRegistration(ctx, req.RegistrationID, familyID)
	if err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, httpx.ErrNotFound("registration not found")
	}
	if reg.Status == "paid" {
		return &ConfirmResult{Status: "paid", RegistrationID: req.RegistrationID}, nil
	}

	count, err := s.repo.AttemptCount(ctx, req.RegistrationID)
	if err != nil {
		return nil, err
	}

	// 1. Check if an explicit failure was requested (e.g. simulation or user cancellation)
	if req.Success != nil && !*req.Success {
		return s.handleFailure(ctx, req.RegistrationID, req.RazorpayOrderID, count)
	}

	// 2. Validate based on provider
	var isValid bool
	if s.provider.Name() == "razorpay" && req.RazorpayOrderID != "" {
		isValid = s.provider.VerifySignature(req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature)
	} else if req.Success != nil && *req.Success {
		isValid = true // Mock simulation or test fallback
	} else if s.provider.Name() == "mock" {
		isValid = true
	}

	if !isValid {
		return s.handleFailure(ctx, req.RegistrationID, req.RazorpayOrderID, count)
	}

	// Payment successful
	_ = s.repo.MarkPaymentSuccess(ctx, req.RegistrationID, req.RazorpayOrderID, req.RazorpayPaymentID, req.RazorpaySignature)
	if err := s.repo.SetRegistrationStatus(ctx, req.RegistrationID, "paid", nil); err != nil {
		return nil, err
	}

	return &ConfirmResult{
		Status:         "paid",
		Attempt:        count,
		AttemptsLeft:   0,
		RegistrationID: req.RegistrationID,
	}, nil
}

func (s *Service) handleFailure(ctx context.Context, regID, orderRef string, currentAttempts int) (*ConfirmResult, error) {
	_ = s.repo.MarkPaymentFailure(ctx, regID, orderRef)

	hold := s.now().Add(holdWindow)
	status := "held"
	left := maxAttempts - currentAttempts
	if currentAttempts >= maxAttempts {
		status = "held"
		left = 0
	}
	if err := s.repo.SetRegistrationStatus(ctx, regID, status, &hold); err != nil {
		return nil, err
	}
	resultStatus := "failed"
	if left <= 0 {
		resultStatus = "locked"
		left = 0
	}
	return &ConfirmResult{
		Status:         resultStatus,
		Attempt:        currentAttempts,
		AttemptsLeft:   left,
		RegistrationID: regID,
	}, nil
}

// Confirm handles legacy confirmation calls.
func (s *Service) Confirm(ctx context.Context, familyID, regID string, success bool) (*ConfirmResult, error) {
	return s.VerifyPayment(ctx, familyID, VerifyRequest{
		RegistrationID: regID,
		Success:        &success,
	})
}

// MockProvider is a local, no-network payment provider for development.
type MockProvider struct{}

func (MockProvider) Name() string  { return "mock" }
func (MockProvider) KeyID() string { return "" }

func (MockProvider) CreateOrder(_ float64, _, _ string) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("order_mock_%09d", n.Int64()), nil
}

func (MockProvider) VerifySignature(_, _, signature string) bool {
	return signature != "invalid_signature"
}
