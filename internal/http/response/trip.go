package response

import (
	"time"

	"github.com/oapi-codegen/nullable"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

func Trip(t trip.Trip) api.Trip {
	finishedAt := nullable.NewNullNullable[time.Time]()
	if t.FinishedAt != nil {
		finishedAt = nullable.NewNullableWithValue(*t.FinishedAt)
	}

	return api.Trip{
		Id:             t.ID,
		UserId:         t.UserID,
		DriverId:       t.DriverID,
		StartPoint:     api.Coordinates{Latitude: t.Start.Latitude, Longitude: t.Start.Longitude},
		EndPoint:       api.Coordinates{Latitude: t.End.Latitude, Longitude: t.End.Longitude},
		Price:          t.Price,
		Status:         api.TripStatus(t.Status),
		StartedAt:      t.StartedAt,
		FinishedAt:     finishedAt,
		LastPositionAt: nullable.NewNullNullable[time.Time](),
	}
}
