package position

import (
	"context"
	"fmt"
	"time"

	sq "github.com/Masterminds/squirrel"
	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

const tablePositions = "trip_positions"

type Repo struct {
	querier      querierProvider
	psql         sq.StatementBuilderType
	queryTimeout time.Duration
}

func New(querier querierProvider, queryTimeout time.Duration) *Repo {
	return &Repo{
		querier:      querier,
		psql:         sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		queryTimeout: queryTimeout,
	}
}

func (r *Repo) Save(ctx context.Context, p trip.Position) (trip.Position, error) {
	query, args, err := r.psql.Insert(tablePositions).
		Columns("trip_id", "latitude", "longitude", "recorded_at").
		Values(p.TripID, p.Point.Latitude, p.Point.Longitude, p.RecordedAt).
		Suffix("RETURNING id").
		ToSql()
	if err != nil {
		return trip.Position{}, fmt.Errorf("build insert position: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	if err := r.querier.Querier(ctx).QueryRow(ctx, query, args...).Scan(&p.ID); err != nil {
		return trip.Position{}, fmt.Errorf("insert position: %w", err)
	}

	return p, nil
}

func (r *Repo) List(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	query, args, err := r.psql.Select("id", "trip_id", "latitude", "longitude", "recorded_at").
		From(tablePositions).
		Where(sq.Eq{"trip_id": tripID}).
		OrderBy("recorded_at", "id").
		ToSql()
	if err != nil {
		return nil, fmt.Errorf("build select positions: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, r.queryTimeout)
	defer cancel()

	rows, err := r.querier.Querier(ctx).Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("select positions: %w", err)
	}
	defer rows.Close()

	positions := []trip.Position{}

	for rows.Next() {
		var p trip.Position
		if err := rows.Scan(&p.ID, &p.TripID, &p.Point.Latitude, &p.Point.Longitude, &p.RecordedAt); err != nil {
			return nil, fmt.Errorf("scan position: %w", err)
		}

		p.RecordedAt = p.RecordedAt.UTC()
		positions = append(positions, p)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("select positions: %w", err)
	}

	return positions, nil
}
