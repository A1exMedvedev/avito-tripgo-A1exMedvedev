package trip

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

func (r *Repo) Fetch(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	query, args, err := r.psql.Select(tripColumns...).
		From(tableTrips).
		Where(sq.Eq{"id": id}).
		ToSql()
	if err != nil {
		return trip.Trip{}, fmt.Errorf("build select trip: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	t, err := scanTrip(r.querier.Querier(ctx).QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return trip.Trip{}, trip.ErrTripNotFound
	}

	if err != nil {
		return trip.Trip{}, fmt.Errorf("select trip: %w", err)
	}

	return t, nil
}

func (r *Repo) FetchIdempotencyKey(ctx context.Context, key uuid.UUID) (model.IdempotencyKey, error) {
	query, args, err := r.psql.Select("key", "request_hash", "trip_id", "created_at").
		From(tableIdempotency).
		Where(sq.Eq{"key": key}).
		ToSql()
	if err != nil {
		return model.IdempotencyKey{}, fmt.Errorf("build select idempotency key: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	var k model.IdempotencyKey

	err = r.querier.Querier(ctx).QueryRow(ctx, query, args...).Scan(&k.Key, &k.RequestHash, &k.TripID, &k.CreatedAt)
	if err != nil {
		return model.IdempotencyKey{}, fmt.Errorf("select idempotency key: %w", err)
	}

	return k, nil
}

func (r *Repo) CountActive(ctx context.Context) (int64, error) {
	query, args, err := r.psql.Select("count(*)").
		From(tableTrips).
		Where(sq.Eq{columnStatus: trip.StatusActive}).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("build count active trips: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	var count int64

	if err := r.querier.Querier(ctx).QueryRow(ctx, query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count active trips: %w", err)
	}

	return count, nil
}
