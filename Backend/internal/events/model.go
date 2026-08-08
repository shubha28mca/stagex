// Package events exposes the participant-facing event catalogue: listing with
// filters (Discover screen) and single-event detail with its categories.
//
// Layering: model (data shapes) → repository (persistence, behind an interface)
// → service (business rules) → controller (HTTP) → routes (wiring). This same
// five-file shape is repeated by every domain package so the codebase is easy
// to navigate and reuse.
package events

import "time"

// Event is a single event as shown on a Discover card and the detail page.
type Event struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Tagline       string    `json:"tagline"`
	City          string    `json:"city"`
	Mode          string    `json:"mode"`   // onstage | online
	Rounds        int       `json:"rounds"`
	Fee           float64   `json:"fee"`
	SlotsTotal    int       `json:"slotsTotal"`
	SlotsFilled   int       `json:"slotsFilled"`
	StartDate     time.Time `json:"startDate"`
	EndDate       time.Time `json:"endDate"`
	Status        string    `json:"status"`
	CoverGradient string    `json:"coverGradient"`
	EventType     string    `json:"eventType"`
	Categories    []EventCategory `json:"categories,omitempty"`
	RoundsDetail  []Round     `json:"roundsDetail,omitempty"`
	Rubric        []Criterion `json:"rubric,omitempty"`
	Judges        []string    `json:"judges,omitempty"`
}

// Round is one named round in an event (Judging wizard step 2).
type Round struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Criterion is one judging-rubric line with its weight percentage.
type Criterion struct {
	Criterion string `json:"criterion"`
	Weight    int    `json:"weight"`
}

// EventCategory is one category offered within an event, with its age band.
type EventCategory struct {
	ID                string  `json:"id"`
	CategoryName      string  `json:"categoryName"`
	AgeBandCode       string  `json:"ageBandCode"`
	AgeBandLabel      string  `json:"ageBandLabel"`
	MinAge            int     `json:"minAge"`
	MaxAge            int     `json:"maxAge"`
	ParticipationType string  `json:"participationType"`
	Fee               float64 `json:"fee"`
}

// Filter captures the Discover search/filter query (ClientDesignWeb §4).
type Filter struct {
	Query   string // event name contains
	City    string
	Mode    string
	MaxFee  float64 // 0 = no cap
	Rounds  int     // 0 = any
	Status  string  // default 'open'
}
