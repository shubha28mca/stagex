package coupons

import (
	"context"
	"math"
	"strings"
	"time"
)

// Service validates a coupon against the order and computes the discount. The
// pure calculation is split out (ComputeDiscount) so it can be unit-tested with
// no database and reused by the registrations flow.
type Service struct {
	repo Repository
	now  func() time.Time // injectable clock for deterministic tests
}

// NewService builds a coupons Service using the real clock.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Validate looks up the code and returns the discount for the given subtotal
// and event. A never-erroring result with Valid=false carries the reason so the
// frontend can show an inline message ("expired", "not valid for this event").
func (s *Service) Validate(ctx context.Context, code string, subtotal float64, eventID string) (ValidationResult, error) {
	code = strings.ToUpper(strings.TrimSpace(code))
	res := ValidationResult{Code: code, Total: subtotal}
	if code == "" {
		res.Reason = "enter a coupon code"
		return res, nil
	}
	c, err := s.repo.GetByCode(ctx, code)
	if err != nil {
		return res, err
	}
	if c == nil {
		res.Reason = "coupon not found"
		return res, nil
	}
	if reason := s.eligibility(c, eventID); reason != "" {
		res.Reason = reason
		return res, nil
	}
	discount := ComputeDiscount(c, subtotal)
	res.Valid = true
	res.Discount = discount
	res.Total = round2(subtotal - discount)
	return res, nil
}

// eligibility returns an empty string when the coupon may be used, otherwise a
// human-readable reason why not.
func (s *Service) eligibility(c *Coupon, eventID string) string {
	now := s.now()
	switch {
	case !c.IsActive:
		return "coupon is not active"
	case now.Before(c.ValidFrom):
		return "coupon is not yet valid"
	case c.ValidUntil != nil && now.After(*c.ValidUntil):
		return "coupon has expired"
	case c.MaxUses > 0 && c.UsedCount >= c.MaxUses:
		return "coupon usage limit reached"
	case c.Scope == "event" && c.ScopeRefID != "" && c.ScopeRefID != eventID:
		return "not valid for this event"
	}
	return ""
}

// ComputeDiscount is the pure pricing rule. It never returns more than the
// subtotal (you cannot go below zero) and rounds to two decimals.
func ComputeDiscount(c *Coupon, subtotal float64) float64 {
	var d float64
	switch c.DiscountType {
	case "percent":
		d = subtotal * (c.Value / 100.0)
	case "flat":
		d = c.Value
	case "sponsored_100":
		d = subtotal
	}
	if d > subtotal {
		d = subtotal
	}
	if d < 0 {
		d = 0
	}
	return round2(d)
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
