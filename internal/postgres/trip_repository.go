package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/trip"
)

const (
	pgUniqueViolation = "23505"

	driverActiveTripIndex = "trips_driver_id_active_uniq"
)

var psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", "status", "started_at", "finished_at",
}

type TripRepository struct {
	pool         *pgxpool.Pool
	queryTimeout time.Duration
}

func NewTripRepository(pool *pgxpool.Pool, queryTimeout time.Duration) *TripRepository {
	return &TripRepository{pool: pool, queryTimeout: queryTimeout}
}

func (r *TripRepository) Create(ctx context.Context, t trip.Trip) error {
	query, args, err := psql.Insert("trips").
		Columns(tripColumns...).
		Values(
			t.ID, t.UserID, t.DriverID,
			t.Start.Latitude, t.Start.Longitude, t.End.Latitude, t.End.Longitude,
			t.Price, t.Status, t.StartedAt, t.FinishedAt,
		).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := executorFrom(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == driverActiveTripIndex {
			return trip.ErrDriverBusy
		}

		return fmt.Errorf("insert trip: %w", err)
	}

	return nil
}

func (r *TripRepository) Get(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	query, args, err := psql.Select(tripColumns...).
		From("trips").
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	t, err := scanTrip(executorFrom(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrTripNotFound
	}

	if err != nil {
		return trip.Trip{}, fmt.Errorf("select trip: %w", err)
	}

	return t, nil
}

func (r *TripRepository) FinishActive(ctx context.Context, id uuid.UUID, at time.Time) (trip.Trip, bool, error) {
	query, args, err := psql.Update("trips").
		Set("status", trip.StatusCompleted).
		Set("finished_at", at).
		Set("updated_at", at).
		Where(sq.Eq{"id": id, "status": trip.StatusActive}).
		Suffix("RETURNING " + strings.Join(tripColumns, ", ")).
		ToSql()
	if err != nil {
		return trip.Trip{}, false, fmt.Errorf("build finish trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	t, err := scanTrip(executorFrom(ctx, r.pool).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, false, nil
	}

	if err != nil {
		return trip.Trip{}, false, fmt.Errorf("finish trip: %w", err)
	}

	return t, true, nil
}

func (r *TripRepository) AddStatusChange(ctx context.Context, c trip.StatusChange) error {
	query, args, err := psql.Insert("trip_status_history").
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(c.TripID, c.From, c.To, c.Reason, c.ChangedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status change: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := executorFrom(ctx, r.pool).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status change: %w", err)
	}

	return nil
}

func (r *TripRepository) ClaimIdempotencyKey(ctx context.Context, k trip.IdempotencyKey, expiredBefore time.Time) (bool, error) {
	query, args, err := psql.Insert("idempotency_keys").
		Columns("key", "request_hash", "trip_id", "created_at").
		Values(k.Key, k.RequestHash, k.TripID, k.CreatedAt).
		Suffix(`ON CONFLICT (key) DO UPDATE
			SET request_hash = EXCLUDED.request_hash, trip_id = EXCLUDED.trip_id, created_at = EXCLUDED.created_at
			WHERE idempotency_keys.created_at < ?
			RETURNING key`, expiredBefore).
		ToSql()
	if err != nil {
		return false, fmt.Errorf("build claim idempotency key: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	var key uuid.UUID
	err = executorFrom(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("claim idempotency key: %w", err)
	}

	return true, nil
}

func (r *TripRepository) GetIdempotencyKey(ctx context.Context, key uuid.UUID) (trip.IdempotencyKey, error) {
	query, args, err := psql.Select("key", "request_hash", "trip_id", "created_at").
		From("idempotency_keys").
		Where(sq.Eq{"key": key}).
		ToSql()
	if err != nil {
		return trip.IdempotencyKey{}, fmt.Errorf("build select idempotency key: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	var k trip.IdempotencyKey
	err = executorFrom(ctx, r.pool).QueryRow(ctx, query, args...).Scan(&k.Key, &k.RequestHash, &k.TripID, &k.CreatedAt)
	if err != nil {
		return trip.IdempotencyKey{}, fmt.Errorf("select idempotency key: %w", err)
	}

	return k, nil
}

func scanTrip(row pgx.Row) (trip.Trip, error) {
	var t trip.Trip

	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.Start.Latitude, &t.Start.Longitude, &t.End.Latitude, &t.End.Longitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		return trip.Trip{}, err
	}

	t.StartedAt = t.StartedAt.UTC()
	if t.FinishedAt != nil {
		t.FinishedAt = new(t.FinishedAt.UTC())
	}

	return t, nil
}
