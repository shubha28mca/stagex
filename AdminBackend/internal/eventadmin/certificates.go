package eventadmin

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Certificate is a result the Event Admin declares for a participant. Position
// is gold (1st) / silver (2nd) / bronze (3rd) / participation. The optional
// image (data URL or link) is shown on the participant's Certificates page.
type Certificate struct {
	ID           string `json:"id"`
	PersonID     string `json:"personId"`
	PersonName   string `json:"personName"`
	CategoryName string `json:"categoryName"`
	Position     string `json:"position"`
	CertCode     string `json:"certCode"`
	FileURL      string `json:"fileUrl"`
}

type issueCertRequest struct {
	PersonID     string `json:"personId"`
	Position     string `json:"position"`     // gold | silver | bronze | participation
	CategoryName string `json:"categoryName"` // optional; derived from the entry if empty
	ImageURL     string `json:"imageUrl"`     // optional; data URL or link
}

var validPositions = map[string]bool{"gold": true, "silver": true, "bronze": true, "participation": true}

type certService struct{ pool *pgxpool.Pool }

func (s certService) list(ctx context.Context, adminID, eventID string) ([]Certificate, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	rows, err := s.pool.Query(ctx, `
		SELECT ct.id, ct.person_id, p.name, ct.category_name, ct.position, ct.cert_code, COALESCE(ct.file_url,'')
		FROM certificates ct JOIN people p ON p.id = ct.person_id
		WHERE ct.event_id=$1 ORDER BY ct.issued_at DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Certificate{}
	for rows.Next() {
		var c Certificate
		if err := rows.Scan(&c.ID, &c.PersonID, &c.PersonName, &c.CategoryName, &c.Position, &c.CertCode, &c.FileURL); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s certService) issue(ctx context.Context, adminID, eventID string, req issueCertRequest) (*Certificate, error) {
	owns, err := eventOwnedBy(ctx, s.pool, adminID, eventID)
	if err != nil {
		return nil, err
	}
	if !owns {
		return nil, httpx.ErrNotFound("event not found or not yours")
	}
	req.Position = strings.ToLower(strings.TrimSpace(req.Position))
	if !validPositions[req.Position] {
		return nil, httpx.ErrBadRequest("position must be gold, silver, bronze or participation")
	}

	// The person must actually be registered in this event; also derive the
	// category name from their entry when the admin didn't supply one.
	var derivedCategory string
	err = s.pool.QueryRow(ctx, `
		SELECT c.name FROM entries en
		JOIN registrations r ON r.id = en.registration_id
		JOIN event_categories ec ON ec.id = en.event_category_id
		JOIN admin_categories c ON c.id = ec.category_id
		WHERE en.person_id=$1 AND r.event_id=$2 LIMIT 1`, req.PersonID, eventID).Scan(&derivedCategory)
	if err == pgx.ErrNoRows {
		return nil, httpx.ErrBadRequest("that participant is not registered in this event")
	}
	if err != nil {
		return nil, err
	}
	category := req.CategoryName
	if strings.TrimSpace(category) == "" {
		category = derivedCategory
	}

	c := Certificate{PersonID: req.PersonID, CategoryName: category, Position: req.Position, CertCode: certCode(), FileURL: req.ImageURL}
	if err := s.pool.QueryRow(ctx, `
		INSERT INTO certificates (person_id, event_id, category_name, position, cert_code, file_url)
		VALUES ($1,$2,$3,$4,$5,NULLIF($6,''))
		RETURNING id`, req.PersonID, eventID, category, req.Position, c.CertCode, req.ImageURL).Scan(&c.ID); err != nil {
		return nil, err
	}
	return &c, nil
}

func (s certService) remove(ctx context.Context, adminID, certID string) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM certificates ct USING events e
		WHERE ct.id=$1 AND ct.event_id=e.id AND e.created_by=$2`, certID, adminID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrNotFound("certificate not found or not yours")
	}
	return nil
}

func registerCertificates(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := certService{pool}
	mux.Handle("GET /admin/event/events/{id}/certificates", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.list(r.Context(), id.AdminID, r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/event/events/{id}/certificates", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var req issueCertRequest
		if err := httpx.Decode(r, &req); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.issue(r.Context(), id.AdminID, r.PathValue("id"), req)
		respondCreated(w, out, err)
	})))
	mux.Handle("DELETE /admin/event/certificates/{certId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		respond(w, map[string]bool{"deleted": true}, svc.remove(r.Context(), id.AdminID, r.PathValue("certId")))
	})))
}

func certCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(900000))
	return fmt.Sprintf("CERT-%06d", n.Int64()+100000)
}
