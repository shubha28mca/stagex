package payments

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Provider abstracts the payment gateway. The mock implementation lets the flow
// run locally; a Razorpay-backed Provider can replace it without touching the
// service.
type Provider interface {
	Name() string
	CreateOrder(amount float64) (orderRef string, err error)
}

// Service enforces the payment business rules (hold window, 3-attempt cap).
type Service struct {
	repo     Repository
	provider Provider
	now      func() time.Time
}

// NewService builds a payments Service.
func NewService(repo Repository, provider Provider) *Service {
	return &Service{repo: repo, provider: provider, now: time.Now}
}

// CreateOrder starts a new payment attempt for a pending registration. It
// refuses to create more than the allowed number of attempts.
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
	orderRef, err := s.provider.CreateOrder(reg.Total)
	if err != nil {
		return nil, httpx.ErrInternal("could not create payment order")
	}
	attempt := count + 1
	if err := s.repo.CreatePayment(ctx, regID, orderRef, reg.Total, attempt, "created"); err != nil {
		return nil, err
	}
	return &Order{OrderRef: orderRef, Amount: reg.Total, Attempt: attempt, Provider: s.provider.Name()}, nil
}

// Confirm records the outcome of the latest attempt. On success the
// registration is marked paid; on failure the entries are held for 30 minutes
// and, once the attempt cap is hit, the registration is locked.
func (s *Service) Confirm(ctx context.Context, familyID, regID string, success bool) (*ConfirmResult, error) {
	reg, err := s.repo.GetRegistration(ctx, regID, familyID)
	if err != nil {
		return nil, err
	}
	if reg == nil {
		return nil, httpx.ErrNotFound("registration not found")
	}
	if reg.Status == "paid" {
		return &ConfirmResult{Status: "paid", RegistrationID: regID}, nil
	}
	count, err := s.repo.AttemptCount(ctx, regID)
	if err != nil {
		return nil, err
	}

	if success {
		_ = s.repo.MarkPaymentStatus(ctx, regID, "", "success")
		if err := s.repo.SetRegistrationStatus(ctx, regID, "paid", nil); err != nil {
			return nil, err
		}
		return &ConfirmResult{Status: "paid", Attempt: count, AttemptsLeft: 0, RegistrationID: regID}, nil
	}

	// Failure: hold entries and lock after the cap.
	hold := s.now().Add(holdWindow)
	status := "held"
	left := maxAttempts - count
	if count >= maxAttempts {
		status = "held" // still held for the 30-min window, but retries are locked
		left = 0
	}
	if err := s.repo.SetRegistrationStatus(ctx, regID, status, &hold); err != nil {
		return nil, err
	}
	resultStatus := "failed"
	if left == 0 {
		resultStatus = "locked"
	}
	return &ConfirmResult{Status: resultStatus, Attempt: count, AttemptsLeft: left, RegistrationID: regID}, nil
}

// MockProvider is a local, no-network payment provider for development.
type MockProvider struct{}

// Name returns the provider name.
func (MockProvider) Name() string { return "mock" }

// CreateOrder returns a random order reference without contacting a gateway.
func (MockProvider) CreateOrder(_ float64) (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("order_mock_%09d", n.Int64()), nil
}
