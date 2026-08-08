package people

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NewPerson carries the fully-prepared values the repository will insert,
// including the already-encrypted Aadhaar bytes and its masked form.
type NewPerson struct {
	FamilyID      string
	Name          string
	DOB           time.Time
	Gender        string
	AadhaarEnc    []byte
	AadhaarMasked string
	Relationship  string
	School        string
	City          string
	Guru          string
}

// Repository persists people scoped to a family.
type Repository interface {
	List(ctx context.Context, familyID string) ([]Person, error)
	GetByID(ctx context.Context, id, familyID string) (*Person, error)
	Create(ctx context.Context, p NewPerson) (*Person, error)
	Update(ctx context.Context, id, familyID string, u updateRequest) (*Person, error)
	// ActiveEventAttachments counts entries for this person in events that have
	// not yet completed — the condition that forces a soft delete.
	ActiveEventAttachments(ctx context.Context, id, familyID string) (int, error)
	// AnyReferences counts all entries + certificates referencing this person,
	// which decides whether a hard delete is possible at all (FK integrity).
	AnyReferences(ctx context.Context, id string) (int, error)
	SoftDelete(ctx context.Context, id, familyID string) error
	HardDelete(ctx context.Context, id, familyID string) error
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewPgRepository builds a Postgres people repository.
func NewPgRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool} }

const selectCols = `id, family_id, name, dob, gender,
	COALESCE(aadhaar_masked,''), relationship,
	COALESCE(school,''), COALESCE(city,''), COALESCE(guru,''),
	COALESCE(photo_url,''), COALESCE(bio,''), (deleted_at IS NOT NULL), created_at`

func (r *pgRepository) List(ctx context.Context, familyID string) ([]Person, error) {
	// Soft-deleted people are still returned (grayed-out in the UI) and sorted
	// after active ones.
	rows, err := r.pool.Query(ctx,
		`SELECT `+selectCols+` FROM people WHERE family_id=$1
		 ORDER BY (deleted_at IS NOT NULL), created_at`, familyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Person
	for rows.Next() {
		p, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *pgRepository) GetByID(ctx context.Context, id, familyID string) (*Person, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT `+selectCols+` FROM people WHERE id=$1 AND family_id=$2`, id, familyID)
	p, err := scan(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *pgRepository) Create(ctx context.Context, p NewPerson) (*Person, error) {
	row := r.pool.QueryRow(ctx, `
		INSERT INTO people (family_id, name, dob, gender, aadhaar_enc, aadhaar_masked,
		                    relationship, school, city, guru)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,''),NULLIF($9,''),NULLIF($10,''))
		RETURNING `+selectCols,
		p.FamilyID, p.Name, p.DOB, p.Gender, p.AadhaarEnc, p.AadhaarMasked,
		p.Relationship, p.School, p.City, p.Guru)
	return scan(row)
}

func (r *pgRepository) Update(ctx context.Context, id, familyID string, u updateRequest) (*Person, error) {
	// COALESCE keeps the existing value when the request field is nil. A nil DOB
	// leaves the date untouched; a provided one re-derives the age band.
	row := r.pool.QueryRow(ctx, `
		UPDATE people SET
			name         = COALESCE($3, name),
			dob          = COALESCE($4::date, dob),
			relationship = COALESCE($5, relationship),
			school       = COALESCE($6, school),
			city         = COALESCE($7, city),
			guru         = COALESCE($8, guru),
			bio          = COALESCE($9, bio),
			photo_url    = COALESCE($10, photo_url),
			updated_at   = now()
		WHERE id=$1 AND family_id=$2 AND deleted_at IS NULL
		RETURNING `+selectCols,
		id, familyID, u.Name, u.DOB, u.Relationship, u.School, u.City, u.Guru, u.Bio, u.Photo)
	p, err := scan(row)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	return p, err
}

func (r *pgRepository) ActiveEventAttachments(ctx context.Context, id, familyID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM entries en
		JOIN registrations r ON r.id = en.registration_id
		JOIN events e ON e.id = r.event_id
		WHERE en.person_id=$1 AND r.family_id=$2 AND e.status <> 'completed'`,
		id, familyID).Scan(&n)
	return n, err
}

func (r *pgRepository) AnyReferences(ctx context.Context, id string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM entries WHERE person_id=$1)
		     + (SELECT COUNT(*) FROM certificates WHERE person_id=$1)`,
		id).Scan(&n)
	return n, err
}

func (r *pgRepository) SoftDelete(ctx context.Context, id, familyID string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE people SET deleted_at=now(), updated_at=now()
		 WHERE id=$1 AND family_id=$2 AND deleted_at IS NULL`, id, familyID)
	return err
}

func (r *pgRepository) HardDelete(ctx context.Context, id, familyID string) error {
	_, err := r.pool.Exec(ctx,
		`DELETE FROM people WHERE id=$1 AND family_id=$2`, id, familyID)
	return err
}

// rowScanner unifies pgx.Row and pgx.Rows for the shared scan helper.
type rowScanner interface {
	Scan(dest ...any) error
}

func scan(row rowScanner) (*Person, error) {
	var p Person
	if err := row.Scan(&p.ID, &p.FamilyID, &p.Name, &p.DOB, &p.Gender,
		&p.AadhaarMasked, &p.Relationship, &p.School, &p.City, &p.Guru,
		&p.PhotoURL, &p.Bio, &p.Deleted, &p.CreatedAt); err != nil {
		return nil, err
	}
	p.AgeYears = ageFromDOB(p.DOB)
	return &p, nil
}

// ageFromDOB computes completed years as of today.
func ageFromDOB(dob time.Time) int {
	now := time.Now()
	years := now.Year() - dob.Year()
	if now.YearDay() < dob.YearDay() {
		years--
	}
	if years < 0 {
		years = 0
	}
	return years
}
