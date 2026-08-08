// Package coupons validates Ops-created coupon codes and computes the discount
// for an order. Coupons live in the admin_coupons master table; participants
// only ever read/validate them (ClientDesignWeb §5.2, rule 5).
package coupons

import "time"

// Coupon mirrors a row in admin_coupons.
type Coupon struct {
	Code         string
	DiscountType string // percent | flat | sponsored_100
	Value        float64
	Scope        string // global | event | category
	ScopeRefID   string
	MaxUses      int
	UsedCount    int
	ValidFrom    time.Time
	ValidUntil   *time.Time
	IsActive     bool
}

// ValidationResult is returned to the client after applying a coupon.
type ValidationResult struct {
	Code     string  `json:"code"`
	Valid    bool    `json:"valid"`
	Reason   string  `json:"reason,omitempty"`
	Discount float64 `json:"discount"`
	Total    float64 `json:"total"`
}
