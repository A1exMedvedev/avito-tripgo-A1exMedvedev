//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}
package positions

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type (
	tripSource interface {
		ListStale(ctx context.Context, staleAfter time.Duration) ([]model.StaleTrip, error)
	}

	pusher interface {
		RequestPosition(ctx context.Context, driverID, tripID uuid.UUID) error
	}

	clock interface {
		Tick(d time.Duration) (ticks <-chan time.Time, stop func())
	}

	logger interface {
		InfoContext(ctx context.Context, msg string, args ...any)
		WarnContext(ctx context.Context, msg string, args ...any)
	}
)
