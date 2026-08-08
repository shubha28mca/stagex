package coupons

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository reads coupons from the admin master table.
type Repository interface {
	GetByCode(ctx context.Context, code string) (*Coupon, error)
}

type pgRepository struct {
	pool *pgxpool.Pool
}

// NewPgRepository builds a Postgres-backed coupons repository.
func NewPgRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

func (r *pgRepository) GetByCode(ctx context.Context, code string) (*Coupon, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT code, discount_type, value, scope, COALESCE(scope_ref_id::text,''),
		       max_uses, used_count, valid_from, valid_until, is_active
		FROM admin_coupons WHERE code = $1`, code)
	var c Coupon
	if err := row.Scan(&c.Code, &c.DiscountType, &c.Value, &c.Scope, &c.ScopeRefID,
		&c.MaxUses, &c.UsedCount, &c.ValidFrom, &c.ValidUntil, &c.IsActive); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}
