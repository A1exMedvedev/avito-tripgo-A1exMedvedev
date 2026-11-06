//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package finish

import (
	"context"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type (
	tripRepository interface {
		Fetch(ctx context.Context, id uuid.UUID) (trip.Trip, error)
		Finish(ctx context.Context, t trip.Trip) error
		SaveStatusChange(ctx context.Context, c model.StatusChange) error
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		TripCompleted(ctx context.Context)
	}
)
