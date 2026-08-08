package events

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository is the persistence contract for events. Defining it as an
// interface lets the service be unit-tested with an in-memory fake and lets us
// swap the storage engine without touching business logic.
type Repository interface {
	List(ctx context.Context, f Filter) ([]Event, error)
	GetByID(ctx context.Context, id string) (*Event, error)
	ListCategories(ctx context.Context, eventID string) ([]EventCategory, error)
}

// pgRepository is the Postgres-backed implementation.
type pgRepository struct {
	pool *pgxpool.Pool
}

// NewPgRepository builds a Postgres repository.
func NewPgRepository(pool *pgxpool.Pool) Repository {
	return &pgRepository{pool: pool}
}

func (r *pgRepository) List(ctx context.Context, f Filter) ([]Event, error) {
	// Build the query with positional args; filters are optional and only
	// applied when set, keeping the SQL injection-safe via parameterization.
	q := `
		SELECT e.id, e.name, e.tagline, e.city, e.mode, e.rounds, e.fee,
		       e.slots_total, e.slots_filled, e.start_date, e.end_date,
		       e.status, e.cover_gradient, COALESCE(et.name,'')
		FROM events e
		LEFT JOIN admin_event_types et ON et.id = e.event_type_id
		WHERE 1=1`
	args := []any{}
	add := func(cond string, val any) {
		args = append(args, val)
		q += cond + argPos(len(args))
	}
	if f.Query != "" {
		add(" AND e.name ILIKE '%'||", f.Query)
		q += "||'%'"
	}
	if f.City != "" {
		add(" AND e.city = ", f.City)
	}
	if f.Mode != "" {
		add(" AND e.mode = ", f.Mode)
	}
	if f.MaxFee > 0 {
		add(" AND e.fee <= ", f.MaxFee)
	}
	if f.Rounds > 0 {
		add(" AND e.rounds >= ", f.Rounds)
	}
	status := f.Status
	if status == "" {
		status = "open"
	}
	add(" AND e.status = ", status)
	q += " ORDER BY e.start_date ASC"

	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

func (r *pgRepository) GetByID(ctx context.Context, id string) (*Event, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT e.id, e.name, e.tagline, e.city, e.mode, e.rounds, e.fee,
		       e.slots_total, e.slots_filled, e.start_date, e.end_date,
		       e.status, e.cover_gradient, COALESCE(et.name,''),
		       e.rounds_detail, e.rubric, e.judge_ids
		FROM events e
		LEFT JOIN admin_event_types et ON et.id = e.event_type_id
		WHERE e.id = $1`, id)
	var e Event
	var roundsJSON, rubricJSON, judgesJSON []byte
	if err := row.Scan(&e.ID, &e.Name, &e.Tagline, &e.City, &e.Mode, &e.Rounds,
		&e.Fee, &e.SlotsTotal, &e.SlotsFilled, &e.StartDate, &e.EndDate,
		&e.Status, &e.CoverGradient, &e.EventType,
		&roundsJSON, &rubricJSON, &judgesJSON); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if len(roundsJSON) > 0 {
		_ = json.Unmarshal(roundsJSON, &e.RoundsDetail)
	}
	if len(rubricJSON) > 0 {
		_ = json.Unmarshal(rubricJSON, &e.Rubric)
	}
	// Resolve judge ids to display names from the Ops master pool.
	var judgeIDs []string
	if len(judgesJSON) > 0 {
		_ = json.Unmarshal(judgesJSON, &judgeIDs)
	}
	if len(judgeIDs) > 0 {
		names, err := r.judgeNames(ctx, judgeIDs)
		if err != nil {
			return nil, err
		}
		e.Judges = names
	}
	return &e, nil
}

// judgeNames resolves admin_judges ids to "Name — Expertise" labels.
func (r *pgRepository) judgeNames(ctx context.Context, ids []string) ([]string, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT name, expertise FROM admin_judges WHERE id = ANY($1::uuid[])`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name, expertise string
		if err := rows.Scan(&name, &expertise); err != nil {
			return nil, err
		}
		out = append(out, name+" — "+expertise)
	}
	return out, rows.Err()
}

func (r *pgRepository) ListCategories(ctx context.Context, eventID string) ([]EventCategory, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT ec.id, c.name, ab.code, ab.label, ab.min_age, ab.max_age,
		       ec.participation_type, ec.fee
		FROM event_categories ec
		JOIN admin_categories c ON c.id = ec.category_id
		JOIN admin_age_bands ab ON ab.id = ec.age_band_id
		WHERE ec.event_id = $1
		ORDER BY c.name`, eventID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventCategory
	for rows.Next() {
		var c EventCategory
		if err := rows.Scan(&c.ID, &c.CategoryName, &c.AgeBandCode, &c.AgeBandLabel,
			&c.MinAge, &c.MaxAge, &c.ParticipationType, &c.Fee); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func scanEvents(rows pgx.Rows) ([]Event, error) {
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Name, &e.Tagline, &e.City, &e.Mode, &e.Rounds,
			&e.Fee, &e.SlotsTotal, &e.SlotsFilled, &e.StartDate, &e.EndDate,
			&e.Status, &e.CoverGradient, &e.EventType); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// argPos renders a positional placeholder like $1 for pgx.
func argPos(n int) string {
	return "$" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
