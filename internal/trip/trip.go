package trip

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

var (
	ErrInvalidTrip   = errors.New("invalid trip")
	ErrTripNotFound  = errors.New("trip not found")
	ErrTripCompleted = errors.New("trip already completed")
	ErrDriverBusy    = errors.New("driver already has an active trip")

	ErrIdempotencyConflict = errors.New("idempotency key already used with a different request")
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

type StatusChange struct {
	TripID    uuid.UUID
	From      *Status
	To        Status
	Reason    string
	ChangedAt time.Time
}

type IdempotencyKey struct {
	Key         uuid.UUID
	RequestHash []byte
	TripID      uuid.UUID
	CreatedAt   time.Time
}

type CreateParams struct {
	UserID   uuid.UUID
	DriverID uuid.UUID
	Start    Point
	End      Point
	Price    int64
}

func (p CreateParams) Validate() error {
	var errs []error

	if p.UserID == uuid.Nil {
		errs = append(errs, errors.New("user_id must not be empty"))
	}

	if p.DriverID == uuid.Nil {
		errs = append(errs, errors.New("driver_id must not be empty"))
	}

	errs = append(errs, validatePoint("start_point", p.Start), validatePoint("end_point", p.End))

	if p.Price < 0 {
		errs = append(errs, errors.New("price must not be negative"))
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidTrip, err)
	}

	return nil
}

func (p CreateParams) hash() []byte {
	sum := sha256.Sum256(fmt.Appendf(nil, "%s|%s|%v|%v|%v|%v|%d",
		p.UserID, p.DriverID,
		p.Start.Latitude, p.Start.Longitude, p.End.Latitude, p.End.Longitude,
		p.Price,
	))

	return sum[:]
}

func validatePoint(name string, p Point) error {
	if p.Latitude < -90 || p.Latitude > 90 {
		return fmt.Errorf("%s.latitude must be in [-90, 90]", name)
	}

	if p.Longitude < -180 || p.Longitude > 180 {
		return fmt.Errorf("%s.longitude must be in [-180, 180]", name)
	}

	return nil
}
