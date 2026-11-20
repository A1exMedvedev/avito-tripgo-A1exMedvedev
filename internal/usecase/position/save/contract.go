//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package save

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

type (
	tripRepository interface {
		Fetch(ctx context.Context, id uuid.UUID) (trip.Trip, error)
		UpdateLastPosition(ctx context.Context, tripID uuid.UUID, recordedAt time.Time) error
	}

	positionRepository interface {
		Save(ctx context.Context, p trip.Position) (trip.Position, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	positionRequests interface {
		Cancel(tripID uuid.UUID)
	}

	metrics interface {
		PositionSaved(ctx context.Context)
	}
)
