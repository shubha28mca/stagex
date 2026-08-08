package eventadmin

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

var adminPhonePattern = regexp.MustCompile(`^[6-9]\d{9}$`)

type offlineAddRequest struct {
	Name            string `json:"name"`
	DOB             string `json:"dob"` // YYYY-MM-DD
	Gender          string `json:"gender"`
	Aadhaar         string `json:"aadhaar"` // optional; stored masked only
	Phone           string `json:"phone"`   // family account phone
	EventCategoryID string `json:"eventCategoryId"`
}

type offlineService struct{ pool *pgxpool.Pool }

// addOffline registers a participant the Event Admin collected in person and who
// paid offline (cash/transfer): it finds/creates the family account, the person,
// and a paid registration + entry in one transaction (Admin Design §5.4).
func (s offlineService) addOffline(ctx context.Context, adminID string, req offlineAddRequest) (map[string]string, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, httpx.ErrBadRequest("name is required")
	}
	dob, err := time.Parse("2006-01-02", req.DOB)
	if err != nil {
		return nil, httpx.ErrBadRequest("date of birth must be YYYY-MM-DD")
	}
	gender := strings.ToLower(strings.TrimSpace(req.Gender))
	if gender != "male" && gender != "female" && gender != "other" {
		return nil, httpx.ErrBadRequest("gender must be male, female or other")
	}
	if !adminPhonePattern.MatchString(req.Phone) {
		return nil, httpx.ErrBadRequest("enter a valid 10-digit family mobile number")
	}

	// Resolve the event category → event + fee, then confirm ownership.
	var eventID string
	var fee float64
	err = s.pool.QueryRow(ctx,
		`SELECT event_id, fee FROM event_categories WHERE id=$1`, req.EventCategoryID).Scan(&eventID, &fee)
	if err == pgx.ErrNoRows {
		return nil, httpx.ErrBadRequest("unknown event category")
	}
	if err != nil {
		return nil, err
	}
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrForbidden("that category belongs to an event you don't own")
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	// Find or create the family account for this phone.
	var familyID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO families (phone, display_name) VALUES ($1,$2)
		ON CONFLICT (phone) DO UPDATE SET updated_at=now()
		RETURNING id`, req.Phone, req.Name).Scan(&familyID); err != nil {
		return nil, err
	}

	var personID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO people (family_id, name, dob, gender, aadhaar_masked, relationship)
		VALUES ($1,$2,$3,$4,NULLIF($5,''),'other')
		RETURNING id`, familyID, req.Name, dob, gender, maskAadhaar(req.Aadhaar)).Scan(&personID); err != nil {
		return nil, err
	}

	var regID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO registrations (family_id, event_id, status, subtotal, discount, total)
		VALUES ($1,$2,'paid',$3,0,$3)
		RETURNING id`, familyID, eventID, fee).Scan(&regID); err != nil {
		return nil, err
	}

	code := entryCode()
	if _, err := tx.Exec(ctx, `
		INSERT INTO payments (registration_id, provider, order_ref, amount, status, attempt)
		VALUES ($1,'offline',$2,$3,'success',1)`, regID, "OFFLINE-"+code, fee); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO entries (registration_id, person_id, event_category_id, entry_code)
		VALUES ($1,$2,$3,$4)`, regID, personID, req.EventCategoryID, code); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE events SET slots_filled = slots_filled + 1 WHERE id=$1`, eventID); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return map[string]string{"entryCode": code, "personName": req.Name}, nil
}

func registerOffline(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := offlineService{pool}
	mux.Handle("POST /admin/event/events/{id}/participants/offline", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var req offlineAddRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.addOffline(r.Context(), id.AdminID, req)
		respondCreated(w, out, err)
	})))
}

// entryCode returns a human-friendly unique-ish entry code.
func entryCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(90000))
	return fmt.Sprintf("SX-%05d", n.Int64()+10000)
}

// maskAadhaar keeps only the last 4 digits (DPDP: admin never sees the full id).
func maskAadhaar(a string) string {
	var digits strings.Builder
	for _, r := range a {
		if r >= '0' && r <= '9' {
			digits.WriteRune(r)
		}
	}
	d := digits.String()
	if len(d) < 4 {
		return ""
	}
	return "XXXX-XXXX-" + d[len(d)-4:]
}
