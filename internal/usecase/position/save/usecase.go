package save

import (
	"context"
	"fmt"
	"time"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
	tx        txManager
	requests  positionRequests
	metrics   metrics
	now       func() time.Time
}

func New(
	trips tripRepository,
	positions positionRepository,
	tx txManager,
	requests positionRequests,
	m metrics,
	now func() time.Time,
) *Usecase {
	return &Usecase{trips: trips, positions: positions, tx: tx, requests: requests, metrics: m, now: now}
}

func (u *Usecase) Do(ctx context.Context, in model.SavePositionInput) (trip.Position, error) {
	p, err := trip.NewPosition(in.TripID, in.Point, in.RecordedAt, u.now())
	if err != nil {
		return trip.Position{}, err
	}

	err = u.tx.Do(ctx, func(ctx context.Context) error {
		t, err := u.trips.Fetch(ctx, in.TripID)
		if err != nil {
			return err
		}

		if err := t.EnsureActive(); err != nil {
			return err
		}

		if err := u.trips.UpdateLastPosition(ctx, in.TripID, p.RecordedAt); err != nil {
			return err
		}

		p, err = u.positions.Save(ctx, p)

		return err
	})
	if err != nil {
		return trip.Position{}, fmt.Errorf("save position for trip %s: %w", in.TripID, err)
	}

	u.requests.Cancel(in.TripID)
	u.metrics.PositionSaved(ctx)

	return p, nil
}
