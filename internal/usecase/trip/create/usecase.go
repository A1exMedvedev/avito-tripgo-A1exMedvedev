package create

import (
	"bytes"
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
	trips          tripRepository
	keys           idempotencyRepository
	tx             txManager
	metrics        metrics
	log            *slog.Logger
	now            func() time.Time
	idempotencyTTL time.Duration
}

func New(
	trips tripRepository,
	keys idempotencyRepository,
	tx txManager,
	m metrics,
	log *slog.Logger,
	now func() time.Time,
	idempotencyTTL time.Duration,
) *Usecase {
	return &Usecase{
		trips:          trips,
		keys:           keys,
		tx:             tx,
		metrics:        m,
		log:            log,
		now:            now,
		idempotencyTTL: idempotencyTTL,
	}
}

func (u *Usecase) Do(ctx context.Context, in model.CreateTripInput) (model.CreateTripResult, error) {
	id, err := uuid.NewV7()
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
			existing, claimed, err := u.claimIdempotencyKey(ctx, *in.IdempotencyKey, hashParams(in.Params), t)
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

func (u *Usecase) claimIdempotencyKey(ctx context.Context, key uuid.UUID, hash []byte, t trip.Trip) (existingTripID uuid.UUID, claimed bool, err error) {
	claimed, err = u.keys.ClaimIdempotencyKey(ctx, model.IdempotencyKey{
		Key:         key,
		RequestHash: hash,
		TripID:      t.ID,
		CreatedAt:   t.StartedAt,
	}, t.StartedAt.Add(-u.idempotencyTTL))
	if err != nil || claimed {
		return uuid.Nil, claimed, err
	}

	existing, err := u.keys.FetchIdempotencyKey(ctx, key)
	if err != nil {
		return uuid.Nil, false, err
	}

	if !bytes.Equal(existing.RequestHash, hash) {
		return uuid.Nil, false, trip.ErrIdempotencyConflict
	}

	return existing.TripID, false, nil
}

func hashParams(p trip.Params) []byte {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%v|%v|%v|%v|%d",
		p.UserID, p.DriverID,
		p.Start.Latitude, p.Start.Longitude, p.End.Latitude, p.End.Longitude,
		p.Price,
	))

	return sum[:]
}
