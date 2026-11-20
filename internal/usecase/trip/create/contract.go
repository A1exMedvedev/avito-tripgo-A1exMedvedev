//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type (
	tripRepository interface {
		Save(ctx context.Context, t trip.Trip) error
		SaveStatusChange(ctx context.Context, c model.StatusChange) error
		Fetch(ctx context.Context, id uuid.UUID) (trip.Trip, error)
	}

	idempotency interface {
		Claim(ctx context.Context, k model.IdempotencyKey) (existingTripID uuid.UUID, claimed bool, err error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		TripCreated(ctx context.Context)
	}

	logger interface {
		InfoContext(ctx context.Context, msg string, args ...any)
	}
)
