package trip

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type TxManager interface {
	Do(ctx context.Context, fn func(ctx context.Context) error) error
}

type Repository interface {
	Create(ctx context.Context, t Trip) error
	Get(ctx context.Context, id uuid.UUID) (Trip, error)
	FinishActive(ctx context.Context, id uuid.UUID, at time.Time) (t Trip, found bool, err error)
	AddStatusChange(ctx context.Context, c StatusChange) error
	ClaimIdempotencyKey(ctx context.Context, k IdempotencyKey, expiredBefore time.Time) (claimed bool, err error)
	GetIdempotencyKey(ctx context.Context, key uuid.UUID) (IdempotencyKey, error)
}

type Service struct {
	repo           Repository
	tx             TxManager
	now            func() time.Time
	idempotencyTTL time.Duration
}

func NewService(repo Repository, tx TxManager, idempotencyTTL time.Duration) *Service {
	return &Service{
		repo:           repo,
		tx:             tx,
		now:            func() time.Time { return time.Now().UTC().Truncate(time.Microsecond) },
		idempotencyTTL: idempotencyTTL,
	}
}

func (s *Service) Create(ctx context.Context, p CreateParams, idempotencyKey *uuid.UUID) (t Trip, replayed bool, err error) {
	if err := p.Validate(); err != nil {
		return Trip{}, false, err
	}

	id, err := uuid.NewV7()
	if err != nil {
		return Trip{}, false, fmt.Errorf("generate trip id: %w", err)
	}

	t = Trip{
		ID:        id,
		UserID:    p.UserID,
		DriverID:  p.DriverID,
		Start:     p.Start,
		End:       p.End,
		Price:     p.Price,
		Status:    StatusActive,
		StartedAt: s.now(),
	}

	err = s.tx.Do(ctx, func(ctx context.Context) error {
		if idempotencyKey != nil {
			existing, claimed, err := s.claimIdempotencyKey(ctx, *idempotencyKey, p.hash(), t)
			if err != nil {
				return err
			}

			if !claimed {
				t, err = s.repo.Get(ctx, existing)
				replayed = true

				return err
			}
		}

		if err := s.repo.Create(ctx, t); err != nil {
			return err
		}

		return s.repo.AddStatusChange(ctx, StatusChange{
			TripID:    t.ID,
			To:        StatusActive,
			Reason:    "trip created",
			ChangedAt: t.StartedAt,
		})
	})
	if err != nil {
		return Trip{}, false, fmt.Errorf("create trip: %w", err)
	}

	return t, replayed, nil
}

func (s *Service) claimIdempotencyKey(ctx context.Context, key uuid.UUID, hash []byte, t Trip) (existingTripID uuid.UUID, claimed bool, err error) {
	claimed, err = s.repo.ClaimIdempotencyKey(ctx, IdempotencyKey{
		Key:         key,
		RequestHash: hash,
		TripID:      t.ID,
		CreatedAt:   t.StartedAt,
	}, t.StartedAt.Add(-s.idempotencyTTL))
	if err != nil || claimed {
		return uuid.Nil, claimed, err
	}

	existing, err := s.repo.GetIdempotencyKey(ctx, key)
	if err != nil {
		return uuid.Nil, false, err
	}

	if !bytes.Equal(existing.RequestHash, hash) {
		return uuid.Nil, false, ErrIdempotencyConflict
	}

	return existing.TripID, false, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (Trip, error) {
	t, err := s.repo.Get(ctx, id)
	if err != nil {
		return Trip{}, fmt.Errorf("get trip %s: %w", id, err)
	}

	return t, nil
}

func (s *Service) Finish(ctx context.Context, id uuid.UUID) (Trip, error) {
	var t Trip

	err := s.tx.Do(ctx, func(ctx context.Context) error {
		finished, found, err := s.repo.FinishActive(ctx, id, s.now())
		if err != nil {
			return err
		}

		if !found {
			if _, err := s.repo.Get(ctx, id); err != nil {
				return err
			}

			return ErrTripCompleted
		}

		t = finished

		return s.repo.AddStatusChange(ctx, StatusChange{
			TripID:    t.ID,
			From:      new(StatusActive),
			To:        StatusCompleted,
			Reason:    "trip finished",
			ChangedAt: *t.FinishedAt,
		})
	})
	if err != nil {
		return Trip{}, fmt.Errorf("finish trip %s: %w", id, err)
	}

	return t, nil
}
