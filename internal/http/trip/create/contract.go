//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package create

import (
	"context"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type usecase interface {
	Do(ctx context.Context, in model.CreateTripInput) (model.CreateTripResult, error)
}
