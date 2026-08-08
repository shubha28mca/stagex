package payments

import (
	"context"
	"testing"
	"time"
)

// fakeRepo is an in-memory payments store for tests.
type fakeRepo struct {
	reg      *regInfo
	attempts int
	status   string
}

func (f *fakeRepo) GetRegistration(_ context.Context, _, _ string) (*regInfo, error) {
	return f.reg, nil
}
func (f *fakeRepo) AttemptCount(_ context.Context, _ string) (int, error) { return f.attempts, nil }
func (f *fakeRepo) CreatePayment(_ context.Context, _, _ string, _ float64, attempt int, _ string) error {
	f.attempts = attempt
	return nil
}
func (f *fakeRepo) MarkPaymentStatus(_ context.Context, _, _, _ string) error { return nil }
func (f *fakeRepo) SetRegistrationStatus(_ context.Context, _, status string, _ *time.Time) error {
	f.status = status
	if f.reg != nil {
		f.reg.Status = status
	}
	return nil
}

func TestCreateOrderIncrementsAttempt(t *testing.T) {
	repo := &fakeRepo{reg: &regInfo{Total: 499, Status: "pending"}, attempts: 0}
	svc := NewService(repo, MockProvider{})
	order, err := svc.CreateOrder(context.Background(), "fam", "reg")
	if err != nil {
		t.Fatal(err)
	}
	if order.Attempt != 1 || order.Amount != 499 || order.OrderRef == "" {
		t.Fatalf("unexpected order: %+v", order)
	}
}

func TestCreateOrderLocksAfterMaxAttempts(t *testing.T) {
	repo := &fakeRepo{reg: &regInfo{Total: 499, Status: "pending"}, attempts: maxAttempts}
	svc := NewService(repo, MockProvider{})
	if _, err := svc.CreateOrder(context.Background(), "fam", "reg"); err == nil {
		t.Fatal("expected lockout after max attempts")
	}
}

func TestConfirmSuccessMarksPaid(t *testing.T) {
	repo := &fakeRepo{reg: &regInfo{Total: 499, Status: "pending"}, attempts: 1}
	svc := NewService(repo, MockProvider{})
	res, err := svc.Confirm(context.Background(), "fam", "reg", true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "paid" || repo.status != "paid" {
		t.Fatalf("expected paid, got %+v (repo=%s)", res, repo.status)
	}
}

func TestConfirmFailureHoldsAndLocks(t *testing.T) {
	// Below the cap → failed with attempts left.
	repo := &fakeRepo{reg: &regInfo{Total: 499, Status: "pending"}, attempts: 1}
	svc := NewService(repo, MockProvider{})
	res, _ := svc.Confirm(context.Background(), "fam", "reg", false)
	if res.Status != "failed" || res.AttemptsLeft != maxAttempts-1 {
		t.Fatalf("unexpected fail result: %+v", res)
	}
	if repo.status != "held" {
		t.Fatalf("expected entries held, got %s", repo.status)
	}

	// At the cap → locked.
	repo2 := &fakeRepo{reg: &regInfo{Total: 499, Status: "pending"}, attempts: maxAttempts}
	svc2 := NewService(repo2, MockProvider{})
	res2, _ := svc2.Confirm(context.Background(), "fam", "reg", false)
	if res2.Status != "locked" || res2.AttemptsLeft != 0 {
		t.Fatalf("expected locked, got %+v", res2)
	}
}
