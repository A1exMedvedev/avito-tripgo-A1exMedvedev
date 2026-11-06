package create

import (
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

func toInput(body api.TripData, params api.CreateTripParams) model.CreateTripInput {
	return model.CreateTripInput{
		Params: trip.Params{
			UserID:   body.UserId,
			DriverID: body.DriverId,
			Start:    trip.Point{Latitude: body.StartPoint.Latitude, Longitude: body.StartPoint.Longitude},
			End:      trip.Point{Latitude: body.EndPoint.Latitude, Longitude: body.EndPoint.Longitude},
			Price:    body.Price,
		},
		IdempotencyKey: params.IdempotencyKey,
	}
}
