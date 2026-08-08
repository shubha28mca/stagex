package registrations

import (
	"context"
	"testing"
	"time"

	"github.com/iig/stagex/backend/internal/coupons"
)

var fixedNow = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func dob(age int) time.Time {
	return time.Date(fixedNow.Year()-age, 1, 1, 0, 0, 0, 0, time.UTC)
}

func TestEvaluateEligibility(t *testing.T) {
	// Age 10 into a 9-12 band → eligible.
	ok := []entryContext{{
		PersonID: "p1", PersonName: "Diya", PersonDOB: dob(10),
		EventCatID: "ec1", CategoryName: "Classical Dance", EventID: "evt1",
		MinAge: 9, MaxAge: 12, Fee: 499,
	}}
	entries, subtotal, err := evaluate("evt1", ok, fixedNow)
	if err != nil {
		t.Fatalf("expected eligible, got %v", err)
	}
	if len(entries) != 1 || subtotal != 499 {
		t.Fatalf("unexpected entries/subtotal: %v %v", entries, subtotal)
	}
	if entries[0].EntryCode == "" {
		t.Fatal("expected an entry code to be generated")
	}

	// Age 10 into a 13-16 band → not eligible.
	bad := []entryContext{{
		PersonID: "p1", PersonName: "Diya", PersonDOB: dob(10),
		EventCatID: "ec2", CategoryName: "Folk Dance", EventID: "evt1",
		MinAge: 13, MaxAge: 16, Fee: 499,
	}}
	if _, _, err := evaluate("evt1", bad, fixedNow); err == nil {
		t.Fatal("expected ineligible age error")
	}

	// Entry belonging to a different event → error.
	mixed := []entryContext{{
		PersonID: "p1", PersonDOB: dob(10), EventID: "evtX",
		MinAge: 0, MaxAge: 200, Fee: 100,
	}}
	if _, _, err := evaluate("evt1", mixed, fixedNow); err == nil {
		t.Fatal("expected cross-event error")
	}
}

// ---- fakes for Create ----

type fakeRepo struct {
	ctxs    map[string]*entryContext // key: personID+"|"+ecID
	created *Registration
}

func (f *fakeRepo) LoadEntryContext(_ context.Context, _, personID, ecID string) (*entryContext, error) {
	return f.ctxs[personID+"|"+ecID], nil
}
func (f *fakeRepo) Create(_ context.Context, reg *Registration) error {
	reg.ID = "reg-1"
	reg.CreatedAt = fixedNow
	for i := range reg.Entries {
		reg.Entries[i].ID = "entry-" + reg.Entries[i].PersonID
	}
	f.created = reg
	return nil
}

// fakeCoupon implements CouponValidator.
type fakeCoupon struct{ discount float64 }

func (f fakeCoupon) Validate(_ context.Context, code string, subtotal float64, _ string) (coupons.ValidationResult, error) {
	if code == "GOOD" {
		return coupons.ValidationResult{Code: code, Valid: true, Discount: f.discount, Total: subtotal - f.discount}, nil
	}
	return coupons.ValidationResult{Code: code, Valid: false, Reason: "bad code"}, nil
}

func TestCreateWithCoupon(t *testing.T) {
	repo := &fakeRepo{ctxs: map[string]*entryContext{
		"p1|ec1": {PersonID: "p1", PersonName: "Diya", PersonDOB: dob(10), EventCatID: "ec1", CategoryName: "Dance", EventID: "evt1", MinAge: 9, MaxAge: 12, Fee: 499},
		"p2|ec1": {PersonID: "p2", PersonName: "Kabir", PersonDOB: dob(11), EventCatID: "ec1", CategoryName: "Dance", EventID: "evt1", MinAge: 9, MaxAge: 12, Fee: 499},
	}}
	svc := NewService(repo, fakeCoupon{discount: 100})
	svc.now = func() time.Time { return fixedNow }

	reg, err := svc.Create(context.Background(), "fam1", createRequest{
		EventID:    "evt1",
		CouponCode: "GOOD",
		Entries: []createEntryInput{
			{PersonID: "p1", EventCategoryID: "ec1"},
			{PersonID: "p2", EventCategoryID: "ec1"},
		},
	})
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}
	if reg.Subtotal != 998 || reg.Discount != 100 || reg.Total != 898 {
		t.Fatalf("pricing wrong: %+v", reg)
	}
	if len(reg.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(reg.Entries))
	}
}

func TestCreateRejectsBadCoupon(t *testing.T) {
	repo := &fakeRepo{ctxs: map[string]*entryContext{
		"p1|ec1": {PersonID: "p1", PersonName: "Diya", PersonDOB: dob(10), EventCatID: "ec1", CategoryName: "Dance", EventID: "evt1", MinAge: 9, MaxAge: 12, Fee: 499},
	}}
	svc := NewService(repo, fakeCoupon{})
	svc.now = func() time.Time { return fixedNow }
	_, err := svc.Create(context.Background(), "fam1", createRequest{
		EventID:    "evt1",
		CouponCode: "BAD",
		Entries:    []createEntryInput{{PersonID: "p1", EventCategoryID: "ec1"}},
	})
	if err == nil {
		t.Fatal("expected bad coupon to fail the registration")
	}
}

func TestCreateRequiresEntries(t *testing.T) {
	svc := NewService(&fakeRepo{ctxs: map[string]*entryContext{}}, fakeCoupon{})
	if _, err := svc.Create(context.Background(), "fam1", createRequest{EventID: "evt1"}); err == nil {
		t.Fatal("expected error for no entries")
	}
}
