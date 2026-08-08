package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// EventVendor / EventSponsor are partners assigned to an event; their cost is
// income that adds to the event profit.
type EventVendor struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	ServiceType string  `json:"serviceType"`
	Cost        float64 `json:"cost"`
}
type EventSponsor struct {
	ID           string  `json:"id"`
	Organisation string  `json:"organisation"`
	Tier         string  `json:"tier"`
	Cost         float64 `json:"cost"`
}

type partnerService struct{ pool *pgxpool.Pool }

func (s partnerService) listVendors(ctx context.Context, eventID string) ([]EventVendor, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, service_type, cost FROM admin_event_vendors WHERE event_id=$1 ORDER BY name`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventVendor{}
	for rows.Next() {
		var v EventVendor
		if err := rows.Scan(&v.ID, &v.Name, &v.ServiceType, &v.Cost); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s partnerService) assignVendor(ctx context.Context, eventID, vendorID string, cost float64) error {
	if err := (eventOpsService{s.pool}).ensureEvent(ctx, eventID); err != nil {
		return err
	}
	ct, err := s.pool.Exec(ctx, `
		INSERT INTO admin_event_vendors (event_id, vendor_id, name, service_type, cost)
		SELECT $1, id, name, service_type, $3 FROM admin_vendors WHERE id=$2 AND is_active`,
		eventID, vendorID, cost)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrBadRequest("vendor not found or inactive")
	}
	return nil
}

func (s partnerService) listSponsors(ctx context.Context, eventID string) ([]EventSponsor, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, organisation, tier, cost FROM admin_event_sponsors WHERE event_id=$1 ORDER BY organisation`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventSponsor{}
	for rows.Next() {
		var sp EventSponsor
		if err := rows.Scan(&sp.ID, &sp.Organisation, &sp.Tier, &sp.Cost); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

func (s partnerService) assignSponsor(ctx context.Context, eventID, sponsorID string, cost float64) error {
	if err := (eventOpsService{s.pool}).ensureEvent(ctx, eventID); err != nil {
		return err
	}
	ct, err := s.pool.Exec(ctx, `
		INSERT INTO admin_event_sponsors (event_id, sponsor_id, organisation, tier, cost)
		SELECT $1, id, organisation, tier, $3 FROM admin_sponsors WHERE id=$2`,
		eventID, sponsorID, cost)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrBadRequest("sponsor not found")
	}
	return nil
}

func (s partnerService) unassign(ctx context.Context, table, id string) error {
	// table is a fixed internal constant, never user input.
	_, err := s.pool.Exec(ctx, "DELETE FROM "+table+" WHERE id=$1", id)
	return err
}

func registerPartners(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := partnerService{pool}

	mux.Handle("GET /admin/ops/events/{id}/vendors", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.listVendors(r.Context(), r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/events/{id}/vendors/assign", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			VendorID string  `json:"vendorId"`
			Cost     float64 `json:"cost"`
		}
		if err := httpx.Decode(r, &body); err != nil {
			httpx.Error(w, err)
			return
		}
		respondCreated(w, map[string]bool{"assigned": true}, svc.assignVendor(r.Context(), r.PathValue("id"), body.VendorID, body.Cost))
	})))
	mux.Handle("DELETE /admin/ops/event-vendors/{assignmentId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.unassign(r.Context(), "admin_event_vendors", r.PathValue("assignmentId")))
	})))

	mux.Handle("GET /admin/ops/events/{id}/sponsors", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.listSponsors(r.Context(), r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/events/{id}/sponsors/assign", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			SponsorID string  `json:"sponsorId"`
			Cost      float64 `json:"cost"`
		}
		if err := httpx.Decode(r, &body); err != nil {
			httpx.Error(w, err)
			return
		}
		respondCreated(w, map[string]bool{"assigned": true}, svc.assignSponsor(r.Context(), r.PathValue("id"), body.SponsorID, body.Cost))
	})))
	mux.Handle("DELETE /admin/ops/event-sponsors/{assignmentId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.unassign(r.Context(), "admin_event_sponsors", r.PathValue("assignmentId")))
	})))
}
