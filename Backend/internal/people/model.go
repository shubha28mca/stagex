// Package people manages the family's reusable participants (My People). A
// person's mandatory identity fields are name, DOB, gender and Aadhaar; school,
// city and guru are optional and can be added later (ClientDesignWeb §5.1, R1).
//
// Aadhaar is encrypted at rest and only ever returned masked.
package people

import "time"

// Person is a family member. Aadhaar is never serialized in full — only the
// masked form leaves the server.
type Person struct {
	ID            string    `json:"id"`
	FamilyID      string    `json:"familyId"`
	Name          string    `json:"name"`
	DOB           time.Time `json:"dob"`
	AgeYears      int       `json:"ageYears"` // derived from DOB
	Gender        string    `json:"gender"`
	AadhaarMasked string    `json:"aadhaarMasked"`
	Relationship  string    `json:"relationship"`
	School        string    `json:"school,omitempty"`
	City          string    `json:"city,omitempty"`
	Guru          string    `json:"guru,omitempty"`
	PhotoURL      string    `json:"photoUrl,omitempty"`
	Bio           string    `json:"bio,omitempty"`
	Deleted       bool      `json:"deleted"`
	CreatedAt     time.Time `json:"createdAt"`
}

// createRequest is the body for POST /people. Mandatory fields are validated in
// the service; optional fields may be empty.
type createRequest struct {
	Name         string `json:"name"`
	DOB          string `json:"dob"` // YYYY-MM-DD
	Gender       string `json:"gender"`
	Aadhaar      string `json:"aadhaar"`
	Relationship string `json:"relationship"`
	School       string `json:"school"`
	City         string `json:"city"`
	Guru         string `json:"guru"`
}

// updateRequest is the body for PATCH /people/{id}. All fields optional; only
// provided (non-nil) fields are changed. Identity fields that require
// re-verification (gender, Aadhaar) are intentionally not editable here.
type updateRequest struct {
	Name         *string `json:"name"`
	DOB          *string `json:"dob"` // YYYY-MM-DD; re-derives age band eligibility
	Relationship *string `json:"relationship"`
	School       *string `json:"school"`
	City         *string `json:"city"`
	Guru         *string `json:"guru"`
	Bio          *string `json:"bio"`
	Photo        *string `json:"photoUrl"`
}

// DeleteResult tells the client whether the person was fully removed or retained
// in a soft-deleted (grayed-out) state because they are attached to an event
// that has not completed yet.
type DeleteResult struct {
	Removed     bool   `json:"removed"`
	SoftDeleted bool   `json:"softDeleted"`
	Message     string `json:"message"`
}
