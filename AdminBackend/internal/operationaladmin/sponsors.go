package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Sponsor mirrors admin_sponsors.
type Sponsor struct {
	ID               string  `json:"id"`
	Organisation     string  `json:"organisation"`
	Tier             string  `json:"tier"` // platinum | gold | silver | impact
	Contact          string  `json:"contact"`
	CommittedAmount  float64 `json:"committedAmount"`
	ScholarshipSlots int     `json:"scholarshipSlots"`
}

type sponsorService struct{ pool *pgxpool.Pool }

func (s sponsorService) list(ctx context.Context) ([]Sponsor, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, organisation, tier, COALESCE(contact,''), committed_amount, scholarship_slots
		 FROM admin_sponsors ORDER BY organisation`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Sponsor{}
	for rows.Next() {
		var sp Sponsor
		if err := rows.Scan(&sp.ID, &sp.Organisation, &sp.Tier, &sp.Contact,
			&sp.CommittedAmount, &sp.ScholarshipSlots); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s sponsorService) create(ctx context.Context, sp Sponsor) (*Sponsor, error) {
	if sp.Organisation == "" {
		return nil, httpx.ErrBadRequest("organisation is required")
	}
	if sp.Tier == "" {
		sp.Tier = "silver"
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_sponsors (organisation, tier, contact, committed_amount, scholarship_slots)
		VALUES ($1,$2,NULLIF($3,''),$4,$5) RETURNING id`,
		sp.Organisation, sp.Tier, sp.Contact, sp.CommittedAmount, sp.ScholarshipSlots).Scan(&sp.ID)
	if err != nil {
		return nil, err
	}
	return &sp, nil
}

func (s sponsorService) update(ctx context.Context, id string, sp Sponsor) (*Sponsor, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_sponsors SET organisation=$2, tier=$3, contact=NULLIF($4,''),
			committed_amount=$5, scholarship_slots=$6, updated_at=now()
		WHERE id=$1`, id, sp.Organisation, sp.Tier, sp.Contact, sp.CommittedAmount, sp.ScholarshipSlots)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("sponsor not found")
	}
	sp.ID = id
	return &sp, nil
}

func (s sponsorService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_sponsors WHERE id=$1`, id)
	return err
}

func registerSponsors(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := sponsorService{pool}
	mux.Handle("GET /admin/ops/sponsors", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/sponsors", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sp Sponsor
		if err := httpx.Decode(r, &sp); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), sp)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/sponsors/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var sp Sponsor
		if err := httpx.Decode(r, &sp); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), sp)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/sponsors/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}
