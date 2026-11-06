package trip

import (
	"time"

	sq "github.com/Masterminds/squirrel"
)

const (
	tableTrips         = "trips"
	tableStatusHistory = "trip_status_history"
	tableIdempotency   = "idempotency_keys"
)

type Repo struct {
	querier      querierProvider
	psql         sq.StatementBuilderType
	queryTimeout time.Duration
}

func New(querier querierProvider, queryTimeout time.Duration) *Repo {
	return &Repo{
		querier:      querier,
		psql:         sq.StatementBuilder.PlaceholderFormat(sq.Dollar),
		queryTimeout: queryTimeout,
	}
}
