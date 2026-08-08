package auth

import (
	"context"
	"testing"
	"time"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
)

// ---- in-memory fakes ----

type fakeFamilyRepo struct{ byPhone map[string]*Family }

func newFakeFamilyRepo() *fakeFamilyRepo { return &fakeFamilyRepo{byPhone: map[string]*Family{}} }

func (f *fakeFamilyRepo) GetByPhone(_ context.Context, phone string) (*Family, error) {
	fam, ok := f.byPhone[phone]
	if !ok {
		return nil, nil
	}
	cp := *fam // return a copy, mirroring a fresh DB row scan
	return &cp, nil
}
func (f *fakeFamilyRepo) Create(_ context.Context, phone, name, hash string) (*Family, error) {
	fam := &Family{ID: "fam-" + phone, Phone: phone, DisplayName: name, PasswordHash: hash, CreatedAt: time.Now()}
	f.byPhone[phone] = fam
	cp := *fam
	return &cp, nil
}

type fakeOTPRepo struct{ items []*OTPChallenge }

func (f *fakeOTPRepo) Create(_ context.Context, phone, hash, purpose string, exp time.Time) error {
	f.items = append(f.items, &OTPChallenge{ID: "otp", Phone: phone, CodeHash: hash, Purpose: purpose, ExpiresAt: exp})
	return nil
}
func (f *fakeOTPRepo) LatestActive(_ context.Context, phone, purpose string) (*OTPChallenge, error) {
	for i := len(f.items) - 1; i >= 0; i-- {
		c := f.items[i]
		if c.Phone == phone && c.Purpose == purpose && !c.Consumed {
			return c, nil
		}
	}
	return nil, nil
}
func (f *fakeOTPRepo) IncrementAttempts(_ context.Context, id string) error {
	for _, c := range f.items {
		if c.ID == id {
			c.Attempts++
		}
	}
	return nil
}
func (f *fakeOTPRepo) Consume(_ context.Context, id string) error {
	for _, c := range f.items {
		if c.ID == id {
			c.Consumed = true
		}
	}
	return nil
}

func newTestService() (*Service, *fakeFamilyRepo, *fakeOTPRepo) {
	fam := newFakeFamilyRepo()
	otp := &fakeOTPRepo{}
	tokens := platauth.NewManager("test-secret", time.Hour)
	svc := NewService(fam, otp, tokens, 5*time.Minute, true) // devMode → OTP returned
	return svc, fam, otp
}

func TestRegisterAndLogin(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	phone := "9876543210"

	sent, err := svc.SendOTP(ctx, phone, "register")
	if err != nil {
		t.Fatal(err)
	}
	if sent.DevOTP == "" {
		t.Fatal("expected dev otp in dev mode")
	}

	resp, err := svc.Register(ctx, phone, "Priya", "Str0ngPass!", sent.DevOTP)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if resp.Token == "" || resp.Family.Phone != phone {
		t.Fatalf("unexpected register response: %+v", resp)
	}

	// Duplicate registration is rejected.
	if _, err := svc.Register(ctx, phone, "Priya", "Str0ngPass!", "000000"); err == nil {
		t.Fatal("expected duplicate registration to fail")
	}

	// Password login works.
	if _, err := svc.Login(ctx, phone, "Str0ngPass!", ""); err != nil {
		t.Fatalf("password login failed: %v", err)
	}
	// Wrong password rejected.
	if _, err := svc.Login(ctx, phone, "wrong", ""); err == nil {
		t.Fatal("expected wrong password to fail")
	}

	// OTP login works.
	sent2, _ := svc.SendOTP(ctx, phone, "login")
	if _, err := svc.Login(ctx, phone, "", sent2.DevOTP); err != nil {
		t.Fatalf("otp login failed: %v", err)
	}
}

func TestOTPLockout(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	phone := "9811111111"
	if _, err := svc.SendOTP(ctx, phone, "login"); err != nil {
		t.Fatal(err)
	}
	// Three wrong attempts, then the fourth is locked out.
	for i := 0; i < 3; i++ {
		if err := svc.VerifyOTP(ctx, phone, "000000", "login"); err == nil {
			t.Fatalf("attempt %d: expected wrong-otp error", i+1)
		}
	}
	err := svc.VerifyOTP(ctx, phone, "000000", "login")
	if err == nil {
		t.Fatal("expected lockout error")
	}
}

func TestRegisterValidation(t *testing.T) {
	svc, _, _ := newTestService()
	ctx := context.Background()
	if _, err := svc.SendOTP(ctx, "123", "register"); err == nil {
		t.Fatal("expected invalid phone rejection")
	}
	if _, err := svc.Register(ctx, "9876543210", "P", "short", "000000"); err == nil {
		t.Fatal("expected short password rejection")
	}
}
