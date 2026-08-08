package operationaladmin

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Participant is the Ops view of a person across all families.
type Participant struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	DOB           time.Time `json:"dob"`
	Gender        string    `json:"gender"`
	Relationship  string    `json:"relationship"`
	AadhaarMasked string    `json:"aadhaarMasked"`
	City          string    `json:"city"`
	FamilyPhone   string    `json:"familyPhone"`
	Deleted       bool      `json:"deleted"`
}

// participantUpdate is the unrestricted edit Ops may apply to any participant.
type participantUpdate struct {
	Name         string `json:"name"`
	Gender       string `json:"gender"`
	Relationship string `json:"relationship"`
	School       string `json:"school"`
	City         string `json:"city"`
	Guru         string `json:"guru"`
}

type participantService struct{ pool *pgxpool.Pool }

func (s participantService) list(ctx context.Context) ([]Participant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, p.dob, p.gender, p.relationship,
		       COALESCE(p.aadhaar_masked,''), COALESCE(p.city,''), f.phone,
		       (p.deleted_at IS NOT NULL)
		FROM people p JOIN families f ON f.id = p.family_id
		ORDER BY (p.deleted_at IS NOT NULL), p.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Participant{}
	for rows.Next() {
		var p Participant
		if err := rows.Scan(&p.ID, &p.Name, &p.DOB, &p.Gender, &p.Relationship,
			&p.AadhaarMasked, &p.City, &p.FamilyPhone, &p.Deleted); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s participantService) update(ctx context.Context, id string, u participantUpdate) error {
	ct, err := s.pool.Exec(ctx, `
		UPDATE people SET name=$2, gender=$3, relationship=$4,
			school=NULLIF($5,''), city=NULLIF($6,''), guru=NULLIF($7,''), updated_at=now()
		WHERE id=$1`, id, u.Name, u.Gender, u.Relationship, u.School, u.City, u.Guru)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrNotFound("participant not found")
	}
	return nil
}

// del force-removes a participant and their entries/certificates in a
// transaction — Ops has no restriction (per the requirement).
func (s participantService) del(ctx context.Context, id string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	for _, q := range []string{
		`DELETE FROM entries WHERE person_id=$1`,
		`DELETE FROM certificates WHERE person_id=$1`,
		`DELETE FROM people WHERE id=$1`,
	} {
		if _, err := tx.Exec(ctx, q, id); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func registerParticipants(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := participantService{pool}
	mux.Handle("GET /admin/ops/participants", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("PATCH /admin/ops/participants/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var u participantUpdate
		if err := httpx.Decode(r, &u); err != nil {
			httpx.Error(w, err)
			return
		}
		respond(w, map[string]bool{"updated": true}, svc.update(r.Context(), r.PathValue("id"), u))
	})))
	mux.Handle("DELETE /admin/ops/participants/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.del(r.Context(), r.PathValue("id")))
	})))
}

// RegisterRoutes wires every Operational Admin endpoint behind the ops guard.
func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	registerEventTypes(mux, pool, guard)
	registerAgeBands(mux, pool, guard)
	registerCategories(mux, pool, guard)
	registerCoupons(mux, pool, guard)
	registerHalls(mux, pool, guard)
	registerJudges(mux, pool, guard)
	registerSponsors(mux, pool, guard)
	registerOversight(mux, pool, guard)
	registerParticipants(mux, pool, guard)
	registerCrewPool(mux, pool, guard)
	registerEventOps(mux, pool, guard)
	registerVendors(mux, pool, guard)
	registerPartners(mux, pool, guard)
	registerOpsReport(mux, pool, guard)
}
