package payments

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// regInfo is the minimal registration data the payment flow needs.
type regInfo struct {
	Total  float64
	Status string
}

// Repository persists payment attempts and updates registration state.
type Repository interface {
	GetRegistration(ctx context.Context, regID, familyID string) (*regInfo, error)
	AttemptCount(ctx context.Context, regID string) (int, error)
	CreatePayment(ctx context.Context, regID, orderRef string, amount float64, attempt int, status string) error
	MarkPaymentStatus(ctx context.Context, regID, orderRef, status string) error
	SetRegistrationStatus(ctx context.Context, regID, status string, heldUntil *time.Time) error
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewPgRepository builds a Postgres payments repository.
func NewPgRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool} }

func (r *pgRepository) GetRegistration(ctx context.Context, regID, familyID string) (*regInfo, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT total, status FROM registrations WHERE id=$1 AND family_id=$2`, regID, familyID)
	var ri regInfo
	if err := row.Scan(&ri.Total, &ri.Status); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &ri, nil
}

func (r *pgRepository) AttemptCount(ctx context.Context, regID string) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM payments WHERE registration_id=$1`, regID).Scan(&n)
	return n, err
}

func (r *pgRepository) CreatePayment(ctx context.Context, regID, orderRef string, amount float64, attempt int, status string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payments (registration_id, order_ref, amount, attempt, status)
		VALUES ($1,$2,$3,$4,$5)`, regID, orderRef, amount, attempt, status)
	return err
}

func (r *pgRepository) MarkPaymentStatus(ctx context.Context, regID, orderRef, status string) error {
	// When orderRef is empty we update every payment for the registration (used
	// on success confirmation); otherwise we target the specific order.
	_, err := r.pool.Exec(ctx,
		`UPDATE payments SET status=$3 WHERE registration_id=$1 AND ($2='' OR order_ref=$2)`,
		regID, orderRef, status)
	return err
}

func (r *pgRepository) SetRegistrationStatus(ctx context.Context, regID, status string, heldUntil *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE registrations SET status=$2, held_until=$3, updated_at=now() WHERE id=$1`,
		regID, status, heldUntil)
	return err
}
