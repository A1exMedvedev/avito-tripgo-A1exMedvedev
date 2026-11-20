package list

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

type Usecase struct {
	trips     tripRepository
	positions positionRepository
}

func New(trips tripRepository, positions positionRepository) *Usecase {
	return &Usecase{trips: trips, positions: positions}
}

func (u *Usecase) Do(ctx context.Context, tripID uuid.UUID) ([]trip.Position, error) {
	if _, err := u.trips.Fetch(ctx, tripID); err != nil {
		return nil, fmt.Errorf("list positions for trip %s: %w", tripID, err)
	}

	positions, err := u.positions.List(ctx, tripID)
	if err != nil {
		return nil, fmt.Errorf("list positions for trip %s: %w", tripID, err)
	}

	return positions, nil
}
