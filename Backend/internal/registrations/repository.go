package registrations

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository resolves entry contexts and persists registrations + entries.
type Repository interface {
	// LoadEntryContext resolves a person (family-scoped) and an event category
	// into the data needed for eligibility + pricing. Returns nil if either the
	// person or the event category does not exist / is not owned by the family.
	LoadEntryContext(ctx context.Context, familyID, personID, eventCategoryID string) (*entryContext, error)
	// Create persists the registration and its entries in one transaction.
	Create(ctx context.Context, reg *Registration) error
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewPgRepository builds a Postgres registrations repository.
func NewPgRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool} }

func (r *pgRepository) LoadEntryContext(ctx context.Context, familyID, personID, eventCategoryID string) (*entryContext, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT p.id, p.name, p.dob,
		       ec.id, c.name, ec.event_id, ab.min_age, ab.max_age, ec.fee
		FROM people p
		CROSS JOIN event_categories ec
		JOIN admin_categories c ON c.id = ec.category_id
		JOIN admin_age_bands ab ON ab.id = ec.age_band_id
		WHERE p.id = $1 AND p.family_id = $2 AND ec.id = $3 AND p.deleted_at IS NULL`,
		personID, familyID, eventCategoryID)
	var e entryContext
	if err := row.Scan(&e.PersonID, &e.PersonName, &e.PersonDOB,
		&e.EventCatID, &e.CategoryName, &e.EventID, &e.MinAge, &e.MaxAge, &e.Fee); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

func (r *pgRepository) Create(ctx context.Context, reg *Registration) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after successful commit

	if err := tx.QueryRow(ctx, `
		INSERT INTO registrations (family_id, event_id, status, coupon_code, subtotal, discount, total)
		VALUES ($1,$2,'pending',NULLIF($3,''),$4,$5,$6)
		RETURNING id, created_at`,
		reg.FamilyID, reg.EventID, reg.CouponCode, reg.Subtotal, reg.Discount, reg.Total,
	).Scan(&reg.ID, &reg.CreatedAt); err != nil {
		return err
	}

	for i := range reg.Entries {
		en := &reg.Entries[i]
		if err := tx.QueryRow(ctx, `
			INSERT INTO entries (registration_id, person_id, event_category_id, entry_code)
			VALUES ($1,$2,$3,$4)
			RETURNING id`,
			reg.ID, en.PersonID, en.EventCategoryID, en.EntryCode,
		).Scan(&en.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
