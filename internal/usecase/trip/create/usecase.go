package create

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

const reasonCreated = "trip created"

type Usecase struct {
	trips       tripRepository
	idempotency idempotency
	tx          txManager
	metrics     metrics
	log         logger
	now         func() time.Time
	newID       func() (uuid.UUID, error)
}

func New(
	trips tripRepository,
	idem idempotency,
	tx txManager,
	m metrics,
	log logger,
	now func() time.Time,
	newID func() (uuid.UUID, error),
) *Usecase {
	return &Usecase{
		trips:       trips,
		idempotency: idem,
		tx:          tx,
		metrics:     m,
		log:         log,
		now:         now,
		newID:       newID,
	}
}

func (u *Usecase) Do(ctx context.Context, in model.CreateTripInput) (model.CreateTripResult, error) {
	id, err := u.newID()
	if err != nil {
		return model.CreateTripResult{}, fmt.Errorf("generate trip id: %w", err)
	}

	t, err := trip.New(id, in.Params, u.now())
	if err != nil {
		return model.CreateTripResult{}, err
	}

	replayed := false

	err = u.tx.Do(ctx, func(ctx context.Context) error {
		if in.IdempotencyKey != nil {
			existing, claimed, err := u.idempotency.Claim(ctx, model.IdempotencyKey{
				Key:         *in.IdempotencyKey,
				RequestHash: hashParams(in.Params),
				TripID:      t.ID,
				CreatedAt:   t.StartedAt,
			})
			if err != nil {
				return err
			}

			if !claimed {
				t, err = u.trips.Fetch(ctx, existing)
				replayed = true

				return err
			}
		}

		if err := u.trips.Save(ctx, t); err != nil {
			return err
		}

		return u.trips.SaveStatusChange(ctx, model.StatusChange{
			TripID:    t.ID,
			To:        trip.StatusActive,
			Reason:    reasonCreated,
			ChangedAt: t.StartedAt,
		})
	})
	if err != nil {
		return model.CreateTripResult{}, fmt.Errorf("create trip: %w", err)
	}

	if !replayed {
		u.metrics.TripCreated(ctx)
		u.log.InfoContext(ctx, reasonCreated, slog.String("trip_id", t.ID.String()))
	}

	return model.CreateTripResult{Trip: t, Replayed: replayed}, nil
}

func hashParams(p trip.Params) []byte {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%v|%v|%v|%v|%d",
		p.UserID, p.DriverID,
		p.Start.Latitude, p.Start.Longitude, p.End.Latitude, p.End.Longitude,
		p.Price,
	))

	return sum[:]
}
