//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"
	"time"

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

	idempotencyRepository interface {
		ClaimIdempotencyKey(ctx context.Context, k model.IdempotencyKey, expiredBefore time.Time) (claimed bool, err error)
		FetchIdempotencyKey(ctx context.Context, key uuid.UUID) (model.IdempotencyKey, error)
	}

	txManager interface {
		Do(ctx context.Context, fn func(ctx context.Context) error) error
	}

	metrics interface {
		TripCreated(ctx context.Context)
	}
)
