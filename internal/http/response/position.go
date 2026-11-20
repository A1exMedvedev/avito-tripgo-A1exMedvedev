package response

import (
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

func Position(p trip.Position) api.TripPosition {
	return api.TripPosition{
		Id:          p.ID,
		TripId:      p.TripID,
		Coordinates: api.Coordinates{Latitude: p.Point.Latitude, Longitude: p.Point.Longitude},
		RecordedAt:  p.RecordedAt,
	}
}

func Positions(positions []trip.Position) api.TripPositionsResponse {
	out := make([]api.TripPosition, 0, len(positions))
	for _, p := range positions {
		out = append(out, Position(p))
	}

	return api.TripPositionsResponse{Positions: out}
}
