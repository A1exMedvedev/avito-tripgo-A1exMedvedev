package idempotency

import (
	"bytes"
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type Service struct {
	keys repository
	ttl  time.Duration
}

func New(keys repository, ttl time.Duration) *Service {
	return &Service{keys: keys, ttl: ttl}
}

func (s *Service) Claim(ctx context.Context, k model.IdempotencyKey) (existingTripID uuid.UUID, claimed bool, err error) {
	claimed, err = s.keys.ClaimIdempotencyKey(ctx, k, k.CreatedAt.Add(-s.ttl))
	if err != nil || claimed {
		return uuid.Nil, claimed, err
	}

	existing, err := s.keys.FetchIdempotencyKey(ctx, k.Key)
	if err != nil {
		return uuid.Nil, false, err
	}

	if !bytes.Equal(existing.RequestHash, k.RequestHash) {
		return uuid.Nil, false, trip.ErrIdempotencyConflict
	}

	return existing.TripID, false, nil
}
