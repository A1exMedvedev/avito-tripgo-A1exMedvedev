package trip

import (
	"context"
	"errors"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

const (
	pgUniqueViolation = "23505"

	driverActiveTripIndex = "trips_driver_id_active_uniq"
)

func (r *Repo) Save(ctx context.Context, t trip.Trip) error {
	query, args, err := r.psql.Insert(tableTrips).
		Columns(tripColumns...).
		Values(tripValues(t)...).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := r.querier.Querier(ctx).Exec(ctx, query, args...); err != nil {
		if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == pgUniqueViolation && pgErr.ConstraintName == driverActiveTripIndex {
			return trip.ErrDriverBusy
		}

		return fmt.Errorf("insert trip: %w", err)
	}

	return nil
}

func (r *Repo) Finish(ctx context.Context, t trip.Trip) error {
	query, args, err := r.psql.Update(tableTrips).
		Set(columnStatus, t.Status).
		Set("finished_at", t.FinishedAt).
		Set("updated_at", t.FinishedAt).
		Where(sq.Eq{"id": t.ID, columnStatus: trip.StatusActive}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build finish trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	tag, err := r.querier.Querier(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("finish trip: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return trip.ErrTripCompleted
	}

	return nil
}

func (r *Repo) SaveStatusChange(ctx context.Context, c model.StatusChange) error {
	query, args, err := r.psql.Insert(tableStatusHistory).
		Columns("trip_id", "from_status", "to_status", "reason", "changed_at").
		Values(c.TripID, c.From, c.To, c.Reason, c.ChangedAt).
		ToSql()
	if err != nil {
		return fmt.Errorf("build insert status change: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if _, err := r.querier.Querier(ctx).Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("insert status change: %w", err)
	}

	return nil
}

func (r *Repo) ClaimIdempotencyKey(ctx context.Context, k model.IdempotencyKey, expiredBefore time.Time) (bool, error) {
	query, args, err := r.psql.Insert(tableIdempotency).
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

	err = r.querier.Querier(ctx).QueryRow(ctx, query, args...).Scan(&key)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("claim idempotency key: %w", err)
	}

	return true, nil
}
