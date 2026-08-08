// Package myevents returns the family's registrations for the "My Events"
// screen: each registration with its event summary and per-participant entries
// (ClientDesignWeb §6).
package myevents

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	platauth "github.com/iig/stagex/backend/internal/platform/auth"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// RegisteredEvent is one event the family has registered for.
type RegisteredEvent struct {
	RegistrationID string       `json:"registrationId"`
	EventID        string       `json:"eventId"`
	EventName      string       `json:"eventName"`
	City           string       `json:"city"`
	StartDate      time.Time    `json:"startDate"`
	Status         string       `json:"status"` // registration status
	EventStatus    string       `json:"eventStatus"`
	Total          float64      `json:"total"`
	Entries        []EntryBrief `json:"entries"`
	Media          []MediaItem  `json:"media"`        // photos/videos from the Event Admin
	Winners        []Winner     `json:"winners"`      // event-wide podium
	Certificates   []CertItem   `json:"certificates"` // the family's own certificates
}

// MediaItem is a photo/video the Event Admin uploaded for the event.
type MediaItem struct {
	Kind string `json:"kind"`
	URL  string `json:"url"`
}

// Winner is a podium result visible to all participants of the event.
type Winner struct {
	PersonName string `json:"personName"`
	Position   string `json:"position"`
}

// CertItem is one of the family's own certificates for the event.
type CertItem struct {
	PersonName string `json:"personName"`
	Position   string `json:"position"`
	CertCode   string `json:"certCode"`
	FileURL    string `json:"fileUrl,omitempty"`
}

// EntryBrief is a participant entry within a registered event.
type EntryBrief struct {
	EntryID      string `json:"entryId"`
	PersonName   string `json:"personName"`
	CategoryName string `json:"categoryName"`
	EntryCode    string `json:"entryCode"`
	Note         string `json:"note,omitempty"`
	PhotoURL     string `json:"eventPhotoUrl,omitempty"`
}

// Service reads the family's registrations.
type Service struct{ pool *pgxpool.Pool }

// NewService builds a myevents Service.
func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// List returns every registration for the family, newest first, each with its
// entries attached.
func (s *Service) List(ctx context.Context, familyID string) ([]RegisteredEvent, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT r.id, e.id, e.name, e.city, e.start_date, r.status, e.status, r.total
		FROM registrations r
		JOIN events e ON e.id = r.event_id
		WHERE r.family_id = $1
		ORDER BY r.created_at DESC`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []RegisteredEvent{}
	index := map[string]int{}
	eventIndex := map[string][]int{} // eventID -> positions in out
	for rows.Next() {
		var re RegisteredEvent
		if err := rows.Scan(&re.RegistrationID, &re.EventID, &re.EventName, &re.City,
			&re.StartDate, &re.Status, &re.EventStatus, &re.Total); err != nil {
			return nil, err
		}
		re.Entries = []EntryBrief{}
		re.Media = []MediaItem{}
		re.Winners = []Winner{}
		re.Certificates = []CertItem{}
		index[re.RegistrationID] = len(out)
		eventIndex[re.EventID] = append(eventIndex[re.EventID], len(out))
		out = append(out, re)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return out, nil
	}
	eventIDs := make([]string, 0, len(eventIndex))
	for id := range eventIndex {
		eventIDs = append(eventIDs, id)
	}

	// Attach entries in a second query to avoid N+1 while keeping the code clear.
	entryRows, err := s.pool.Query(ctx, `
		SELECT en.registration_id, en.id, p.name, c.name, en.entry_code,
		       COALESCE(en.note,''), COALESCE(en.event_photo_url,'')
		FROM entries en
		JOIN registrations r ON r.id = en.registration_id
		JOIN people p ON p.id = en.person_id
		JOIN event_categories ec ON ec.id = en.event_category_id
		JOIN admin_categories c ON c.id = ec.category_id
		WHERE r.family_id = $1`, familyID)
	if err != nil {
		return nil, err
	}
	defer entryRows.Close()
	for entryRows.Next() {
		var regID string
		var eb EntryBrief
		if err := entryRows.Scan(&regID, &eb.EntryID, &eb.PersonName, &eb.CategoryName,
			&eb.EntryCode, &eb.Note, &eb.PhotoURL); err != nil {
			return nil, err
		}
		if i, ok := index[regID]; ok {
			out[i].Entries = append(out[i].Entries, eb)
		}
	}
	if err := entryRows.Err(); err != nil {
		return nil, err
	}

	// Media uploaded by the Event Admin, visible to all participants.
	mediaRows, err := s.pool.Query(ctx,
		`SELECT event_id, kind, url FROM admin_event_media WHERE event_id = ANY($1::uuid[]) ORDER BY created_at DESC`, eventIDs)
	if err == nil {
		for mediaRows.Next() {
			var evID string
			var m MediaItem
			if err := mediaRows.Scan(&evID, &m.Kind, &m.URL); err != nil {
				mediaRows.Close()
				return nil, err
			}
			for _, i := range eventIndex[evID] {
				out[i].Media = append(out[i].Media, m)
			}
		}
		mediaRows.Close()
	}

	// Event-wide podium (visible to everyone).
	winnerRows, err := s.pool.Query(ctx, `
		SELECT ct.event_id, p.name, ct.position FROM certificates ct
		JOIN people p ON p.id = ct.person_id
		WHERE ct.event_id = ANY($1::uuid[]) AND ct.position IN ('gold','silver','bronze')
		ORDER BY CASE ct.position WHEN 'gold' THEN 1 WHEN 'silver' THEN 2 ELSE 3 END`, eventIDs)
	if err == nil {
		for winnerRows.Next() {
			var evID string
			var wn Winner
			if err := winnerRows.Scan(&evID, &wn.PersonName, &wn.Position); err != nil {
				winnerRows.Close()
				return nil, err
			}
			for _, i := range eventIndex[evID] {
				out[i].Winners = append(out[i].Winners, wn)
			}
		}
		winnerRows.Close()
	}

	// The family's own certificates for these events (with the viewable link).
	certRows, err := s.pool.Query(ctx, `
		SELECT ct.event_id, p.name, ct.position, ct.cert_code, COALESCE(ct.file_url,'')
		FROM certificates ct JOIN people p ON p.id = ct.person_id
		WHERE p.family_id = $1 AND ct.event_id = ANY($2::uuid[])`, familyID, eventIDs)
	if err == nil {
		for certRows.Next() {
			var evID string
			var c CertItem
			if err := certRows.Scan(&evID, &c.PersonName, &c.Position, &c.CertCode, &c.FileURL); err != nil {
				certRows.Close()
				return nil, err
			}
			for _, i := range eventIndex[evID] {
				out[i].Certificates = append(out[i].Certificates, c)
			}
		}
		certRows.Close()
	}

	return out, nil
}

// Controller adapts HTTP to the myevents Service.
type Controller struct{ svc *Service }

// NewController builds a myevents Controller.
func NewController(svc *Service) *Controller { return &Controller{svc: svc} }

// RegisterRoutes wires the protected My Events endpoint.
func RegisterRoutes(mux *http.ServeMux, c *Controller, protect func(http.Handler) http.Handler) {
	mux.Handle("GET /api/my/events", protect(http.HandlerFunc(c.list)))
}

func (c *Controller) list(w http.ResponseWriter, r *http.Request) {
	id, _ := platauth.FromContext(r.Context())
	data, err := c.svc.List(r.Context(), id.AccountID)
	if err != nil {
		httpx.Error(w, err)
		return
	}
	httpx.JSON(w, http.StatusOK, data)
}
