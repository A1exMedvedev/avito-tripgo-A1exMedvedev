package trip

import (
	"time"

	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Point struct {
	Latitude  float64
	Longitude float64
}

type Trip struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	DriverID   uuid.UUID
	Start      Point
	End        Point
	Price      int64
	Status     Status
	StartedAt  time.Time
	FinishedAt *time.Time
}

type Params struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    Point
	End      Point
	Price    int64
}

func New(id uuid.UUID, p Params, startedAt time.Time) (Trip, error) {
	if err := p.Validate(); err != nil {
		return Trip{}, err
	}

	return Trip{
		ID:        id,
		UserID:    p.UserID,
		DriverID:  p.DriverID,
		Start:     p.Start,
		End:       p.End,
		Price:     p.Price,
		Status:    StatusActive,
		StartedAt: startedAt,
	}, nil
}

func (t *Trip) Finish(at time.Time) error {
	if t.Status != StatusActive {
		return ErrTripCompleted
	}

	t.Status = StatusCompleted
	t.FinishedAt = &at

	return nil
}

func (p Params) Validate() error {
	var fields []FieldError

	if p.UserID == uuid.Nil {
		fields = append(fields, FieldError{"user_id", "must not be empty"})
	}

	if p.DriverID == uuid.Nil {
		fields = append(fields, FieldError{"driver_id", "must not be empty"})
	}

	fields = append(fields, validatePoint("start_point", p.Start)...)
	fields = append(fields, validatePoint("end_point", p.End)...)

	if p.Price < 0 {
		fields = append(fields, FieldError{"price", "must not be negative"})
	}

	if len(fields) > 0 {
		return &ValidationError{Fields: fields}
	}

	return nil
}

func validatePoint(name string, p Point) []FieldError {
	var fields []FieldError

	if p.Latitude < -90 || p.Latitude > 90 {
		fields = append(fields, FieldError{name + ".latitude", "must be in [-90, 90]"})
	}

	if p.Longitude < -180 || p.Longitude > 180 {
		fields = append(fields, FieldError{name + ".longitude", "must be in [-180, 180]"})
	}

	return fields
}
