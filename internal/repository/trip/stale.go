package trip

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

const lastSeenAt = "COALESCE(last_position_at, started_at)"

func (r *Repo) ListStale(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error) {
	query, args, err := r.psql.Select("id", "driver_id", lastSeenAt).
		From(tableTrips).
		Where(sq.Eq{columnStatus: trip.StatusActive}).
		Where(lastSeenAt+" < now() - make_interval(secs => ?)", staleAfter.Seconds()).
		OrderBy(lastSeenAt, "id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select stale trips: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := r.querier.Querier(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}
	defer rows.Close()

	var trips []model.StaleTrip

	for rows.Next() {
		var t model.StaleTrip
		if err := rows.Scan(&t.ID, &t.DriverID, &t.LastSeenAt); err != nil {
			return nil, fmt.Errorf("scan stale trip: %w", err)
		}

		t.LastSeenAt = t.LastSeenAt.UTC()
		trips = append(trips, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select stale trips: %w", err)
	}

	return trips, nil
}

func (r *Repo) UpdateLastPosition(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error {
	query, args, err := r.psql.Update(tableTrips).
		Set("last_position_at", sq.Expr("GREATEST(last_position_at, ?)", recordedAt)).
		Set("updated_at", sq.Expr("now()")).
		Where(sq.Eq{"id": tripID, columnStatus: trip.StatusActive}).
		ToSql()
	if err != nil {
		return fmt.Errorf("build update last position: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	tag, err := r.querier.Querier(ctx).Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update last position: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return trip.ErrTripCompleted
	}

	return nil
}
