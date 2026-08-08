package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// AssignedCrew is a crew member working a specific event (with cost).
type AssignedCrew struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Role    string  `json:"role"`
	Contact string  `json:"contact"`
	Cost    float64 `json:"cost"`
}

// Expense is an additional cost booked against an event, with a comment.
type Expense struct {
	ID        string  `json:"id"`
	Amount    float64 `json:"amount"`
	Comment   string  `json:"comment"`
	CreatedAt string  `json:"createdAt"`
}

// PnL is the per-event profit & loss (Admin Design §4.10). Vendor and sponsor
// assignment costs are treated as income and added to the profit (per request).
type PnL struct {
	Revenue       float64 `json:"revenue"`
	SponsorIncome float64 `json:"sponsorIncome"`
	VendorIncome  float64 `json:"vendorIncome"`
	TotalIncome   float64 `json:"totalIncome"`
	Participants  int     `json:"participants"`
	CrewCost      float64 `json:"crewCost"`
	Expenses      float64 `json:"expenses"`
	HallCost      float64 `json:"hallCost"`
	TotalExpenses float64 `json:"totalExpenses"`
	NetPL         float64 `json:"netPL"`
	Margin        float64 `json:"margin"`
}

type eventOpsService struct{ pool *pgxpool.Pool }

func (s eventOpsService) ensureEvent(ctx context.Context, eventID string) error {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM events WHERE id=$1)`, eventID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return httpx.ErrNotFound("event not found")
	}
	return nil
}

func (s eventOpsService) listCrew(ctx context.Context, eventID string) ([]AssignedCrew, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, name, role, COALESCE(contact,''), cost FROM admin_event_crew WHERE event_id=$1 ORDER BY role, name`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssignedCrew{}
	for rows.Next() {
		var c AssignedCrew
		if err := rows.Scan(&c.ID, &c.Name, &c.Role, &c.Contact, &c.Cost); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// assignCrew copies a pool member (with their cost) onto the event.
func (s eventOpsService) assignCrew(ctx context.Context, eventID, crewID string) error {
	if err := s.ensureEvent(ctx, eventID); err != nil {
		return err
	}
	ct, err := s.pool.Exec(ctx, `
		INSERT INTO admin_event_crew (event_id, name, role, contact, cost, crew_id)
		SELECT $1, name, role, contact, cost, id FROM admin_crew WHERE id=$2 AND is_active`,
		eventID, crewID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return httpx.ErrBadRequest("crew member not found or inactive")
	}
	return nil
}

func (s eventOpsService) unassignCrew(ctx context.Context, assignmentID string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_event_crew WHERE id=$1`, assignmentID)
	return err
}

func (s eventOpsService) listExpenses(ctx context.Context, eventID string) ([]Expense, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, amount, comment, to_char(created_at,'YYYY-MM-DD') FROM admin_event_expenses WHERE event_id=$1 ORDER BY created_at DESC`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Expense{}
	for rows.Next() {
		var e Expense
		if err := rows.Scan(&e.ID, &e.Amount, &e.Comment, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s eventOpsService) addExpense(ctx context.Context, eventID string, e Expense) error {
	if err := s.ensureEvent(ctx, eventID); err != nil {
		return err
	}
	if e.Amount <= 0 {
		return httpx.ErrBadRequest("amount must be greater than zero")
	}
	_, err := s.pool.Exec(ctx,
		`INSERT INTO admin_event_expenses (event_id, amount, comment) VALUES ($1,$2,$3)`,
		eventID, e.Amount, e.Comment)
	return err
}

func (s eventOpsService) deleteExpense(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_event_expenses WHERE id=$1`, id)
	return err
}

// pnl computes revenue, all expense buckets and the resulting profit/loss.
func (s eventOpsService) pnl(ctx context.Context, eventID string) (*PnL, error) {
	if err := s.ensureEvent(ctx, eventID); err != nil {
		return nil, err
	}
	var p PnL
	err := s.pool.QueryRow(ctx, `
		SELECT
			(SELECT COALESCE(SUM(total),0) FROM registrations WHERE event_id=$1 AND status='paid'),
			(SELECT COALESCE(SUM(cost),0) FROM admin_event_sponsors WHERE event_id=$1),
			(SELECT COALESCE(SUM(cost),0) FROM admin_event_vendors WHERE event_id=$1),
			(SELECT COUNT(*) FROM entries en JOIN registrations r ON r.id=en.registration_id WHERE r.event_id=$1),
			(SELECT COALESCE(SUM(cost),0) FROM admin_event_crew WHERE event_id=$1),
			(SELECT COALESCE(SUM(amount),0) FROM admin_event_expenses WHERE event_id=$1),
			(SELECT COALESCE(h.base_rate,0) FROM events e LEFT JOIN admin_halls h ON h.id=e.hall_id WHERE e.id=$1)`,
		eventID).Scan(&p.Revenue, &p.SponsorIncome, &p.VendorIncome, &p.Participants, &p.CrewCost, &p.Expenses, &p.HallCost)
	if err != nil {
		return nil, err
	}
	p.TotalIncome = p.Revenue + p.SponsorIncome + p.VendorIncome
	p.TotalExpenses = p.CrewCost + p.Expenses + p.HallCost
	p.NetPL = p.TotalIncome - p.TotalExpenses
	if p.TotalIncome > 0 {
		p.Margin = p.NetPL / p.TotalIncome * 100
	}
	return &p, nil
}

func registerEventOps(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := eventOpsService{pool}

	mux.Handle("GET /admin/ops/events/{id}/crew", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.listCrew(r.Context(), r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/events/{id}/crew/assign", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			CrewID string `json:"crewId"`
		}
		if err := httpx.Decode(r, &body); err != nil {
			httpx.Error(w, err)
			return
		}
		respondCreated(w, map[string]bool{"assigned": true}, svc.assignCrew(r.Context(), r.PathValue("id"), body.CrewID))
	})))
	mux.Handle("DELETE /admin/ops/event-crew/{assignmentId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.unassignCrew(r.Context(), r.PathValue("assignmentId")))
	})))

	mux.Handle("GET /admin/ops/events/{id}/expenses", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.listExpenses(r.Context(), r.PathValue("id"))
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/events/{id}/expenses", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e Expense
		if err := httpx.Decode(r, &e); err != nil {
			httpx.Error(w, err)
			return
		}
		respondCreated(w, map[string]bool{"added": true}, svc.addExpense(r.Context(), r.PathValue("id"), e))
	})))
	mux.Handle("DELETE /admin/ops/expenses/{expenseId}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.deleteExpense(r.Context(), r.PathValue("expenseId")))
	})))

	mux.Handle("GET /admin/ops/events/{id}/pnl", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.pnl(r.Context(), r.PathValue("id"))
		respond(w, data, err)
	})))
}
