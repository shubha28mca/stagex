package coupons

import (
	"context"
	"testing"
	"time"
)

// fakeRepo is an in-memory coupon store for tests.
type fakeRepo struct{ m map[string]*Coupon }

func (f fakeRepo) GetByCode(_ context.Context, code string) (*Coupon, error) {
	return f.m[code], nil
}

func TestComputeDiscount(t *testing.T) {
	cases := []struct {
		name     string
		coupon   Coupon
		subtotal float64
		want     float64
	}{
		{"percent 20", Coupon{DiscountType: "percent", Value: 20}, 500, 100},
		{"flat 100", Coupon{DiscountType: "flat", Value: 100}, 500, 100},
		{"flat over subtotal capped", Coupon{DiscountType: "flat", Value: 900}, 500, 500},
		{"sponsored 100%", Coupon{DiscountType: "sponsored_100"}, 499, 499},
		{"unknown type", Coupon{DiscountType: "mystery", Value: 50}, 500, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ComputeDiscount(&c.coupon, c.subtotal)
			if got != c.want {
				t.Fatalf("ComputeDiscount = %v, want %v", got, c.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)
	repo := fakeRepo{m: map[string]*Coupon{
		"EARLYBIRD20": {Code: "EARLYBIRD20", DiscountType: "percent", Value: 20, Scope: "global", IsActive: true, ValidUntil: &future},
		"EXPIRED":     {Code: "EXPIRED", DiscountType: "flat", Value: 100, Scope: "global", IsActive: true, ValidUntil: &past},
		"USEDUP":      {Code: "USEDUP", DiscountType: "flat", Value: 100, Scope: "global", IsActive: true, MaxUses: 1, UsedCount: 1},
		"EVENTONLY":   {Code: "EVENTONLY", DiscountType: "flat", Value: 50, Scope: "event", ScopeRefID: "evt-1", IsActive: true},
	}}
	svc := NewService(repo)
	ctx := context.Background()

	t.Run("valid percent", func(t *testing.T) {
		res, err := svc.Validate(ctx, "earlybird20", 500, "evt-x")
		if err != nil {
			t.Fatal(err)
		}
		if !res.Valid || res.Discount != 100 || res.Total != 400 {
			t.Fatalf("unexpected result: %+v", res)
		}
	})
	t.Run("unknown", func(t *testing.T) {
		res, _ := svc.Validate(ctx, "NOPE", 500, "evt-x")
		if res.Valid || res.Reason == "" {
			t.Fatalf("expected invalid with reason, got %+v", res)
		}
	})
	t.Run("expired", func(t *testing.T) {
		res, _ := svc.Validate(ctx, "EXPIRED", 500, "evt-x")
		if res.Valid || res.Reason != "coupon has expired" {
			t.Fatalf("expected expired, got %+v", res)
		}
	})
	t.Run("used up", func(t *testing.T) {
		res, _ := svc.Validate(ctx, "USEDUP", 500, "evt-x")
		if res.Valid || res.Reason != "coupon usage limit reached" {
			t.Fatalf("expected limit reached, got %+v", res)
		}
	})
	t.Run("wrong event", func(t *testing.T) {
		res, _ := svc.Validate(ctx, "EVENTONLY", 500, "evt-2")
		if res.Valid || res.Reason != "not valid for this event" {
			t.Fatalf("expected event scope reject, got %+v", res)
		}
	})
	t.Run("right event", func(t *testing.T) {
		res, _ := svc.Validate(ctx, "EVENTONLY", 500, "evt-1")
		if !res.Valid || res.Discount != 50 {
			t.Fatalf("expected valid, got %+v", res)
		}
	})
}
