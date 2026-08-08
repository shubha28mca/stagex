package operationaladmin

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/iig/stagex/adminbackend/internal/platform/httpx"
)

// Coupon mirrors admin_coupons (the Ops-managed discount pool).
type Coupon struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	DiscountType string  `json:"discountType"` // percent | flat | sponsored_100
	Value        float64 `json:"value"`
	Scope        string  `json:"scope"` // global | event | category
	MaxUses      int     `json:"maxUses"`
	UsedCount    int     `json:"usedCount"`
	IsActive     bool    `json:"isActive"`
}

type couponService struct{ pool *pgxpool.Pool }

func (s couponService) list(ctx context.Context) ([]Coupon, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id, code, discount_type, value, scope, max_uses, used_count, is_active
		 FROM admin_coupons ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Coupon{}
	for rows.Next() {
		var c Coupon
		if err := rows.Scan(&c.ID, &c.Code, &c.DiscountType, &c.Value, &c.Scope,
			&c.MaxUses, &c.UsedCount, &c.IsActive); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s couponService) create(ctx context.Context, c Coupon) (*Coupon, error) {
	if c.Code == "" {
		return nil, httpx.ErrBadRequest("code is required")
	}
	if c.DiscountType == "" {
		c.DiscountType = "percent"
	}
	if c.Scope == "" {
		c.Scope = "global"
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO admin_coupons (code, discount_type, value, scope, max_uses, valid_until)
		VALUES (UPPER($1),$2,$3,$4,$5, now() + interval '90 days')
		RETURNING id, used_count, is_active`,
		c.Code, c.DiscountType, c.Value, c.Scope, c.MaxUses).Scan(&c.ID, &c.UsedCount, &c.IsActive)
	if err != nil {
		return nil, err
	}
	c.Code = upper(c.Code)
	return &c, nil
}

func (s couponService) update(ctx context.Context, id string, c Coupon) (*Coupon, error) {
	ct, err := s.pool.Exec(ctx, `
		UPDATE admin_coupons SET discount_type=$2, value=$3, scope=$4, max_uses=$5, is_active=$6, updated_at=now()
		WHERE id=$1`, id, c.DiscountType, c.Value, c.Scope, c.MaxUses, c.IsActive)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, httpx.ErrNotFound("coupon not found")
	}
	c.ID = id
	return &c, nil
}

func (s couponService) delete(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM admin_coupons WHERE id=$1`, id)
	return err
}

func registerCoupons(mux *http.ServeMux, pool *pgxpool.Pool, guard func(http.Handler) http.Handler) {
	svc := couponService{pool}
	mux.Handle("GET /admin/ops/coupons", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, err := svc.list(r.Context())
		respond(w, data, err)
	})))
	mux.Handle("POST /admin/ops/coupons", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c Coupon
		if err := httpx.Decode(r, &c); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.create(r.Context(), c)
		respondCreated(w, out, err)
	})))
	mux.Handle("PATCH /admin/ops/coupons/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var c Coupon
		if err := httpx.Decode(r, &c); err != nil {
			httpx.Error(w, err)
			return
		}
		out, err := svc.update(r.Context(), r.PathValue("id"), c)
		respond(w, out, err)
	})))
	mux.Handle("DELETE /admin/ops/coupons/{id}", guard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		respond(w, map[string]bool{"deleted": true}, svc.delete(r.Context(), r.PathValue("id")))
	})))
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'z' {
			b[i] = c - 32
		}
	}
	return string(b)
}
