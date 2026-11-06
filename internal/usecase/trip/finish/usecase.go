package finish

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

const reasonFinished = "trip finished"

type Usecase struct {
	trips   tripRepository
	tx      txManager
	metrics metrics
	log     *slog.Logger
	now     func() time.Time
}

func New(trips tripRepository, tx txManager, m metrics, log *slog.Logger, now func() time.Time) *Usecase {
	return &Usecase{trips: trips, tx: tx, metrics: m, log: log, now: now}
}

func (u *Usecase) Do(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	var t trip.Trip

	err := u.tx.Do(ctx, func(ctx context.Context) error {
		var err error

		t, err = u.trips.Fetch(ctx, id)
		if err != nil {
			return err
		}

		if err := t.Finish(u.now()); err != nil {
			return err
		}

		if err := u.trips.Finish(ctx, t); err != nil {
			return err
		}

		return u.trips.SaveStatusChange(ctx, model.StatusChange{
			TripID:    t.ID,
			From:      new(trip.StatusActive),
			To:        trip.StatusCompleted,
			Reason:    reasonFinished,
			ChangedAt: *t.FinishedAt,
		})
	})
	if err != nil {
		return trip.Trip{}, fmt.Errorf("finish trip %s: %w", id, err)
	}

	u.metrics.TripCompleted(ctx)
	u.log.InfoContext(ctx, "trip completed", slog.String("trip_id", t.ID.String()))

	return t, nil
}
