package people

import (
	"context"
	"strings"
	"time"

	"github.com/iig/stagex/backend/internal/platform/crypto"
	"github.com/iig/stagex/backend/internal/platform/httpx"
)

// Service holds the people business rules: mandatory-field validation, Aadhaar
// checksum + encryption, and family-scoping of every read/write.
type Service struct {
	repo   Repository
	cipher *crypto.Cipher
}

// NewService builds a people Service. The cipher encrypts Aadhaar at rest.
func NewService(repo Repository, cipher *crypto.Cipher) *Service {
	return &Service{repo: repo, cipher: cipher}
}

// List returns every person under the family.
func (s *Service) List(ctx context.Context, familyID string) ([]Person, error) {
	list, err := s.repo.List(ctx, familyID)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []Person{}
	}
	return list, nil
}

// Create validates the mandatory identity fields, encrypts the Aadhaar, and
// stores the new person.
func (s *Service) Create(ctx context.Context, familyID string, req createRequest) (*Person, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, httpx.ErrBadRequest("name is required")
	}
	dob, err := time.Parse("2006-01-02", req.DOB)
	if err != nil {
		return nil, httpx.ErrBadRequest("date of birth must be YYYY-MM-DD")
	}
	if dob.After(time.Now()) {
		return nil, httpx.ErrBadRequest("date of birth cannot be in the future")
	}
	gender := strings.ToLower(strings.TrimSpace(req.Gender))
	if gender != "male" && gender != "female" && gender != "other" {
		return nil, httpx.ErrBadRequest("gender must be male, female or other")
	}
	if !crypto.ValidateAadhaar(req.Aadhaar) {
		return nil, httpx.ErrBadRequest("invalid Aadhaar number")
	}
	enc, err := s.cipher.Encrypt([]byte(digitsOnly(req.Aadhaar)))
	if err != nil {
		return nil, httpx.ErrInternal("could not secure Aadhaar")
	}
	relationship := strings.TrimSpace(req.Relationship)
	if relationship == "" {
		relationship = "other"
	}
	return s.repo.Create(ctx, NewPerson{
		FamilyID:      familyID,
		Name:          name,
		DOB:           dob,
		Gender:        gender,
		AadhaarEnc:    enc,
		AadhaarMasked: crypto.Mask(req.Aadhaar),
		Relationship:  relationship,
		School:        strings.TrimSpace(req.School),
		City:          strings.TrimSpace(req.City),
		Guru:          strings.TrimSpace(req.Guru),
	})
}

// Update patches optional profile fields for a person the family owns.
func (s *Service) Update(ctx context.Context, id, familyID string, req updateRequest) (*Person, error) {
	if req.Name != nil && strings.TrimSpace(*req.Name) == "" {
		return nil, httpx.ErrBadRequest("name cannot be empty")
	}
	if req.DOB != nil {
		dob, err := time.Parse("2006-01-02", *req.DOB)
		if err != nil {
			return nil, httpx.ErrBadRequest("date of birth must be YYYY-MM-DD")
		}
		if dob.After(time.Now()) {
			return nil, httpx.ErrBadRequest("date of birth cannot be in the future")
		}
	}
	p, err := s.repo.Update(ctx, id, familyID, req)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, httpx.ErrNotFound("person not found")
	}
	return p, nil
}

// Delete removes a person. If they are attached to any event that has not
// completed, or they carry historical entries/certificates, they are
// soft-deleted (retained, shown grayed-out) so those references stay intact.
// Only a person with no references at all is fully removed.
func (s *Service) Delete(ctx context.Context, id, familyID string) (*DeleteResult, error) {
	p, err := s.repo.GetByID(ctx, id, familyID)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, httpx.ErrNotFound("person not found")
	}

	active, err := s.repo.ActiveEventAttachments(ctx, id, familyID)
	if err != nil {
		return nil, err
	}
	if active > 0 {
		if err := s.repo.SoftDelete(ctx, id, familyID); err != nil {
			return nil, err
		}
		return &DeleteResult{SoftDeleted: true,
			Message: "Retained until their attached event(s) complete; shown as removed until then."}, nil
	}

	refs, err := s.repo.AnyReferences(ctx, id)
	if err != nil {
		return nil, err
	}
	if refs > 0 {
		// Only past (completed) references remain — keep the history, soft delete.
		if err := s.repo.SoftDelete(ctx, id, familyID); err != nil {
			return nil, err
		}
		return &DeleteResult{SoftDeleted: true,
			Message: "Removed. Past event history is retained."}, nil
	}

	if err := s.repo.HardDelete(ctx, id, familyID); err != nil {
		return nil, err
	}
	return &DeleteResult{Removed: true, Message: "Person removed."}, nil
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
