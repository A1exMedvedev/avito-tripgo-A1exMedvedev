//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package save

import (
	"context"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type (
	usecase interface {
		Do(ctx context.Context, in model.SavePositionInput) (trip.Position, error)
	}

	logger interface {
		ErrorContext(ctx context.Context, msg string, args ...any)
	}
)
