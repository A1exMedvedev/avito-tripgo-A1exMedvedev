package get

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

type Usecase struct {
	trips tripRepository
}

func New(trips tripRepository) *Usecase {
	return &Usecase{trips: trips}
}

func (u *Usecase) Do(ctx context.Context, id uuid.UUID) (trip.Trip, error) {
	t, err := u.trips.Fetch(ctx, id)
	if err != nil {
		return trip.Trip{}, fmt.Errorf("get trip %s: %w", id, err)
	}

	return t, nil
}
