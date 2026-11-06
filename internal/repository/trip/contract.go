package trip

import (
	"context"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres/transaction"
)

type querierProvider interface {
	Querier(ctx context.Context) transaction.Querier
}
