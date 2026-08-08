package registrations

import (
	"context"
	"crypto/rand"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/iig/stagex/backend/internal/coupons"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// CouponValidator is the slice of the coupons service this package needs. Using
// a narrow interface keeps the dependency explicit and the service testable.
type CouponValidator interface {
	Validate(ctx context.Context, code string, subtotal float64, eventID string) (coupons.ValidationResult, error)
}

// Service orchestrates a registration: resolve → eligibility → price → persist.
type Service struct {
	repo    Repository
	coupons CouponValidator
	now     func() time.Time
}

// NewService builds a registrations Service.
func NewService(repo Repository, cv CouponValidator) *Service {
	return &Service{repo: repo, coupons: cv, now: time.Now}
}

// Create validates every requested entry, prices the order (applying a coupon
// if given) and persists a pending registration ready for payment.
func (s *Service) Create(ctx context.Context, familyID string, req createRequest) (*Registration, error) {
	if req.EventID == "" {
		return nil, httpx.ErrBadRequest("eventId is required")
	}
	if len(req.Entries) == 0 {
		return nil, httpx.ErrBadRequest("select at least one participant")
	}

	// Resolve each requested entry into a full context.
	contexts := make([]entryContext, 0, len(req.Entries))
	for _, in := range req.Entries {
		ec, err := s.repo.LoadEntryContext(ctx, familyID, in.PersonID, in.EventCategoryID)
		if err != nil {
			return nil, err
		}
		if ec == nil {
			return nil, httpx.ErrBadRequest("unknown participant or category selection")
		}
		contexts = append(contexts, *ec)
	}

	// Pure eligibility + subtotal evaluation (unit-tested independently).
	entries, subtotal, err := evaluate(req.EventID, contexts, s.now())
	if err != nil {
		return nil, err
	}

	// Apply coupon, if any.
	discount := 0.0
	code := strings.ToUpper(strings.TrimSpace(req.CouponCode))
	if code != "" {
		res, cerr := s.coupons.Validate(ctx, code, subtotal, req.EventID)
		if cerr != nil {
			return nil, cerr
		}
		if !res.Valid {
			return nil, httpx.ErrBadRequest("coupon: " + res.Reason)
		}
		discount = res.Discount
	}

	reg := &Registration{
		FamilyID:   familyID,
		EventID:    req.EventID,
		Status:     "pending",
		CouponCode: code,
		Subtotal:   round2(subtotal),
		Discount:   round2(discount),
		Total:      round2(subtotal - discount),
		Entries:    entries,
	}
	if err := s.repo.Create(ctx, reg); err != nil {
		return nil, err
	}
	return reg, nil
}

// evaluate is the pure core: it verifies all entries belong to the same event,
// checks each person's age against the category's age band, generates entry
// codes and sums the subtotal. It has no I/O so it is fully unit-testable.
func evaluate(eventID string, ctxs []entryContext, now time.Time) ([]Entry, float64, error) {
	var subtotal float64
	entries := make([]Entry, 0, len(ctxs))
	for _, c := range ctxs {
		if c.EventID != eventID {
			return nil, 0, httpx.ErrBadRequest("all entries must belong to the same event")
		}
		age := ageAt(c.PersonDOB, now)
		if age < c.MinAge || age > c.MaxAge {
			return nil, 0, httpx.ErrBadRequest(fmt.Sprintf(
				"%s (age %d) is not eligible for %s — allowed ages %d-%d",
				c.PersonName, age, c.CategoryName, c.MinAge, c.MaxAge))
		}
		code, err := entryCode()
		if err != nil {
			return nil, 0, httpx.ErrInternal("could not generate entry code")
		}
		entries = append(entries, Entry{
			PersonID:        c.PersonID,
			PersonName:      c.PersonName,
			EventCategoryID: c.EventCatID,
			CategoryName:    c.CategoryName,
			EntryCode:       code,
			Fee:             c.Fee,
		})
		subtotal += c.Fee
	}
	return entries, subtotal, nil
}

// ageAt returns completed years for dob as of now.
func ageAt(dob, now time.Time) int {
	years := now.Year() - dob.Year()
	if now.YearDay() < dob.YearDay() {
		years--
	}
	if years < 0 {
		years = 0
	}
	return years
}

// entryCode returns a human-friendly unique-ish code like "SX-48213".
func entryCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(90000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SX-%05d", n.Int64()+10000), nil
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }
