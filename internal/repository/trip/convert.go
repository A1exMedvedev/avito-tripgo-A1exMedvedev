package trip

import (
	"github.com/jackc/pgx/v5"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

const columnStatus = "status"

var tripColumns = []string{
	"id", "user_id", "driver_id",
	"start_latitude", "start_longitude", "end_latitude", "end_longitude",
	"price", columnStatus, "started_at", "finished_at",
}

func tripValues(t trip.Trip) []any {
	return []any{
		t.ID, t.UserID, t.DriverID,
		t.Start.Latitude, t.Start.Longitude, t.End.Latitude, t.End.Longitude,
		t.Price, t.Status, t.StartedAt, t.FinishedAt,
	}
}

func scanTrip(row pgx.Row) (trip.Trip, error) {
	var t trip.Trip

	err := row.Scan(
		&t.ID, &t.UserID, &t.DriverID,
		&t.Start.Latitude, &t.Start.Longitude, &t.End.Latitude, &t.End.Longitude,
		&t.Price, &t.Status, &t.StartedAt, &t.FinishedAt,
	)
	if err != nil {
		return trip.Trip{}, err
	}

	t.StartedAt = t.StartedAt.UTC()
	if t.FinishedAt != nil {
		t.FinishedAt = new(t.FinishedAt.UTC())
	}

	return t, nil
}
