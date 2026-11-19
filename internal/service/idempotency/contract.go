//go:generate go tool mockgen -source ${GOFILE} -destination mocks_test.go -package ${GOPACKAGE}_test
package idempotency

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type repository interface {
	ClaimIdempotencyKey(ctx context.Context, k model.IdempotencyKey, expiredBefore time.Time) (claimed bool, err error)
	FetchIdempotencyKey(ctx context.Context, key uuid.UUID) (model.IdempotencyKey, error)
}
