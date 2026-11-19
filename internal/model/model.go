package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

type CreateTripInput struct {
	Params         trip.Params
	IdempotencyKey *uuid.UUID
}

type CreateTripResult struct {
	Trip     trip.Trip
	Replayed bool
}

type StatusChange struct {
	TripID    uuid.UUID
	From      *trip.Status
	To        trip.Status
	Reason    string
	ChangedAt time.Time
}

type IdempotencyKey struct {
	Key         uuid.UUID
	RequestHash []byte
	TripID      uuid.UUID
	CreatedAt   time.Time
}
