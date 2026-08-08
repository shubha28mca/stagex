// Package registrations implements the 3-step registration flow's server side:
// multi-participant selection, per-person eligibility (age band × category)
// enforced before payment, coupon pricing, and entry-code generation
// (ClientDesignWeb §5, rules 4–6).
package registrations

import "time"

// Registration is one checkout for a family against an event.
type Registration struct {
	ID         string    `json:"id"`
	FamilyID   string    `json:"familyId"`
	EventID    string    `json:"eventId"`
	Status     string    `json:"status"`
	CouponCode string    `json:"couponCode,omitempty"`
	Subtotal   float64   `json:"subtotal"`
	Discount   float64   `json:"discount"`
	Total      float64   `json:"total"`
	Entries    []Entry   `json:"entries"`
	CreatedAt  time.Time `json:"createdAt"`
}

// Entry is one person in one event category.
type Entry struct {
	ID              string  `json:"id"`
	PersonID        string  `json:"personId"`
	PersonName      string  `json:"personName"`
	EventCategoryID string  `json:"eventCategoryId"`
	CategoryName    string  `json:"categoryName"`
	EntryCode       string  `json:"entryCode"`
	Fee             float64 `json:"fee"`
}

// entryContext is the resolved data needed to price and eligibility-check one
// requested entry. It is produced by the repository and consumed by the pure
// evaluation logic, which keeps that logic unit-testable without a database.
type entryContext struct {
	PersonID     string
	PersonName   string
	PersonDOB    time.Time
	EventCatID   string
	CategoryName string
	EventID      string
	MinAge       int
	MaxAge       int
	Fee          float64
}

// ---- Request DTOs ----

type createRequest struct {
	EventID    string             `json:"eventId"`
	CouponCode string             `json:"couponCode"`
	Entries    []createEntryInput `json:"entries"`
}

type createEntryInput struct {
	PersonID        string `json:"personId"`
	EventCategoryID string `json:"eventCategoryId"`
}
