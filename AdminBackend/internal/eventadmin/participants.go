package eventadmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/auth"
	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// EventParticipant is a person registered in one of the admin's events.
type EventParticipant struct {
	PersonID     string `json:"personId"`
	Name         string `json:"name"`
	EventName    string `json:"eventName"`
	CategoryName string `json:"categoryName"`
	EntryCode    string `json:"entryCode"`
	PayStatus    string `json:"payStatus"`
	FamilyPhone  string `json:"familyPhone"`
}

type participantEdit struct {
	Name   string `json:"name"`
	City   string `json:"city"`
	School string `json:"school"`
	Guru   string `json:"guru"`
}

type participantService struct{ pool *pgxpool.Pool }

func (s participantService) list(ctx context.Context, adminID string) ([]EventParticipant, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT p.id, p.name, e.name, c.name, en.entry_code, r.status, f.phone
		FROM entries en
		JOIN registrations r ON r.id = en.registration_id
		JOIN events e ON e.id = r.event_id
		JOIN people p ON p.id = en.person_id
		JOIN families f ON f.id = p.family_id
		JOIN event_categories ec ON ec.id = en.event_category_id
		JOIN admin_categories c ON c.id = ec.category_id
		WHERE e.created_by = $1
		ORDER BY e.name, p.name`, adminID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []EventParticipant{}
	for rows.Next() {
		var p EventParticipant
		if err := rows.Scan(&p.PersonID, &p.Name, &p.EventName, &p.CategoryName,
			&p.EntryCode, &p.PayStatus, &p.FamilyPhone); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// update edits a participant, but only if they are registered in one of the
// admin's events (Event Admins can edit participants of their own events).
func (s participantService) update(ctx context.Context, adminID, personID string, e participantEdit) error {
	var allowed bool
	if err := s.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM entries en
			JOIN registrations r ON r.id = en.registration_id
			JOIN events ev ON ev.id = r.event_id
			WHERE en.person_id=$1 AND ev.created_by=$2)`,
		personID, adminID).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return httpx.ErrForbidden("this participant is not registered in your events")
	}
	_, err := s.pool.Exec(ctx, `
		UPDATE people SET name=$2, city=NULLIF($3,''), school=NULLIF($4,''), guru=NULLIF($5,''), updated_at=now()
		WHERE id=$1`, personID, e.Name, e.City, e.School, e.Guru)
	return err
}

func registerParticipants(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := participantService{pool}
	mux.Handle("GET /admin/event/participants", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		data, err := svc.list(r.Context(), id.AdminID)
		respond(w, data, err)
	})))
	mux.Handle("PATCH /admin/event/participants/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var e participantEdit
		if err := httpx.Decode(r, &e); err != nil {
			httpx.Error(w, err)
			return
		}
		respond(w, map[string]bool{"updated": true}, svc.update(r.Context(), id.AdminID, r.PathValue("id"), e))
	})))
}

// Dashboard is the Event Admin's own-events summary.
type Dashboard struct {
	Events        int     `json:"events"`
	Registrations int     `json:"registrations"`
	Revenue       float64 `json:"revenue"`
	Published     int     `json:"published"`
}

func registerDashboard(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	mux.Handle("GET /admin/event/dashboard", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, _ := auth.FromContext(r.Context())
		var d Dashboard
		err := pool.QueryRow(r.Context(), `
			SELECT (SELECT COUNT(*) FROM events WHERE created_by=$1),
			       (SELECT COUNT(*) FROM events WHERE created_by=$1 AND status IN ('open','live','completed')),
			       (SELECT COUNT(*) FROM registrations r JOIN events e ON e.id=r.event_id WHERE e.created_by=$1),
			       (SELECT COALESCE(SUM(r.total),0) FROM registrations r JOIN events e ON e.id=r.event_id WHERE e.created_by=$1 AND r.status='paid')`,
			id.AdminID).Scan(&d.Events, &d.Published, &d.Registrations, &d.Revenue)
		respond(w, d, err)
	})))
}

// RegisterRoutes wires every Event Admin endpoint behind the event guard.
// mediaRoot/mediaPublicBase configure where uploaded media is stored and served.
func RegisterRoutes(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler, mediaRoot, mediaPublicBase string) {
	registerDashboard(mux, pool, guard)
	registerEvents(mux, pool, guard)
	registerCategories(mux, pool, guard)
	registerParticipants(mux, pool, guard)
	registerOffline(mux, pool, guard)
	registerCrew(mux, pool, guard)
	registerNotifications(mux, pool, guard)
	registerCertificates(mux, pool, guard)
	registerReport(mux, pool, guard)
	registerMedia(mux, pool, guard, mediaRoot, mediaPublicBase)
}
