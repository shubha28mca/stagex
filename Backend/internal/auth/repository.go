package auth

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// FamilyRepository persists family accounts.
type FamilyRepository interface {
	GetByPhone(ctx context.Context, phone string) (*Family, error)
	Create(ctx context.Context, phone, name, passwordHash string) (*Family, error)
}

// OTPRepository persists OTP challenges.
type OTPRepository interface {
	Create(ctx context.Context, phone, codeHash, purpose string, expiresAt time.Time) error
	LatestActive(ctx context.Context, phone, purpose string) (*OTPChallenge, error)
	IncrementAttempts(ctx context.Context, id string) error
	Consume(ctx context.Context, id string) error
}

// ---- Postgres implementations ----

type pgFamilyRepo struct{ pool *pgxpool.Pool }

// NewPgFamilyRepository builds a Postgres family repository.
func NewPgFamilyRepository(pool *pgxpool.Pool) FamilyRepository { return &pgFamilyRepo{pool} }

func (r *pgFamilyRepo) GetByPhone(ctx context.Context, phone string) (*Family, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, phone, display_name, password_hash, created_at
		 FROM families WHERE phone = $1`, phone)
	var f Family
	if err := row.Scan(&f.ID, &f.Phone, &f.DisplayName, &f.PasswordHash, &f.CreatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &f, nil
}

func (r *pgFamilyRepo) Create(ctx context.Context, phone, name, passwordHash string) (*Family, error) {
	row := r.pool.QueryRow(ctx,
		`INSERT INTO families (phone, display_name, password_hash)
		 VALUES ($1,$2,$3)
		 RETURNING id, phone, display_name, password_hash, created_at`,
		phone, name, passwordHash)
	var f Family
	if err := row.Scan(&f.ID, &f.Phone, &f.DisplayName, &f.PasswordHash, &f.CreatedAt); err != nil {
		return nil, err
	}
	return &f, nil
}

type pgOTPRepo struct{ pool *pgxpool.Pool }

// NewPgOTPRepository builds a Postgres OTP repository.
func NewPgOTPRepository(pool *pgxpool.Pool) OTPRepository { return &pgOTPRepo{pool} }

func (r *pgOTPRepo) Create(ctx context.Context, phone, codeHash, purpose string, expiresAt time.Time) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO otp_challenges (phone, code_hash, purpose, expires_at)
		 VALUES ($1,$2,$3,$4)`, phone, codeHash, purpose, expiresAt)
	return err
}

func (r *pgOTPRepo) LatestActive(ctx context.Context, phone, purpose string) (*OTPChallenge, error) {
	row := r.pool.QueryRow(ctx,
		`SELECT id, phone, code_hash, purpose, attempts, expires_at, (consumed_at IS NOT NULL)
		 FROM otp_challenges
		 WHERE phone=$1 AND purpose=$2 AND consumed_at IS NULL
		 ORDER BY created_at DESC LIMIT 1`, phone, purpose)
	var c OTPChallenge
	if err := row.Scan(&c.ID, &c.Phone, &c.CodeHash, &c.Purpose, &c.Attempts, &c.ExpiresAt, &c.Consumed); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *pgOTPRepo) IncrementAttempts(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE otp_challenges SET attempts = attempts + 1 WHERE id=$1`, id)
	return err
}

func (r *pgOTPRepo) Consume(ctx context.Context, id string) error {
	_, err := r.pool.Exec(ctx, `UPDATE otp_challenges SET consumed_at = now() WHERE id=$1`, id)
	return err
}
