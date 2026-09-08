package payments

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// regInfo is the registration data needed for payment order creation & checkout.
type regInfo struct {
	Total       float64
	Status      string
	EventName   string
	FamilyName  string
	FamilyPhone string
}

// Repository persists payment attempts and updates registration state.
type Repository interface {
	GetRegistration(ctx context.Context, regID, familyID string) (*regInfo, error)
	AttemptCount(ctx context.Context, regID string) (int, error)
	CreatePayment(ctx context.Context, regID, provider, orderRef string, amount float64, attempt int, status string) error
	MarkPaymentSuccess(ctx context.Context, regID, orderRef, paymentID, signature string) error
	MarkPaymentFailure(ctx context.Context, regID, orderRef string) error
	SetRegistrationStatus(ctx context.Context, regID, status string, heldUntil *time.Time) error
}

type pgRepository struct{ pool *pgxpool.Pool }

// NewPgRepository builds a Postgres payments repository.
func NewPgRepository(pool *pgxpool.Pool) Repository { return &pgRepository{pool} }

func (r *pgRepository) GetRegistration(ctx context.Context, regID, familyID string) (*regInfo, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT r.total, r.status, COALESCE(e.name, ''), COALESCE(f.display_name, ''), COALESCE(f.phone, '')
		FROM registrations r
		JOIN events e ON e.id = r.event_id
		JOIN families f ON f.id = r.family_id
		WHERE r.id = $1 AND r.family_id = $2`, regID, familyID)

	var ri regInfo
	if err := row.Scan(&ri.Total, &ri.Status, &ri.EventName, &ri.FamilyName, &ri.FamilyPhone); err != nil {
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

func (r *pgRepository) CreatePayment(ctx context.Context, regID, provider, orderRef string, amount float64, attempt int, status string) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO payments (registration_id, provider, order_ref, amount, attempt, status)
		VALUES ($1,$2,$3,$4,$5,$6)`, regID, provider, orderRef, amount, attempt, status)
	return err
}

func (r *pgRepository) MarkPaymentSuccess(ctx context.Context, regID, orderRef, paymentID, signature string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payments 
		SET status='success', payment_id=$3, signature=$4
		WHERE registration_id=$1 AND (order_ref=$2 OR $2='')`,
		regID, orderRef, paymentID, signature)
	return err
}

func (r *pgRepository) MarkPaymentFailure(ctx context.Context, regID, orderRef string) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE payments 
		SET status='failed'
		WHERE registration_id=$1 AND (order_ref=$2 OR $2='')`,
		regID, orderRef)
	return err
}

func (r *pgRepository) SetRegistrationStatus(ctx context.Context, regID, status string, heldUntil *time.Time) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE registrations SET status=$2, held_until=$3, updated_at=now() WHERE id=$1`,
		regID, status, heldUntil)
	return err
}
