package trip

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
)

type Status string

const (
	StatusActive    Status = "active"
	StatusCompleted Status = "completed"
)

type Point struct {
	Latitude  float64 `field:"latitude"  validate:"gte=-90,lte=90"`
	Longitude float64 `field:"longitude" validate:"gte=-180,lte=180"`
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
	UserID   uuid.UUID `field:"user_id"     validate:"required"`
	DriverID uuid.UUID `field:"driver_id"   validate:"required"`
	Start    Point     `field:"start_point"`
	End      Point     `field:"end_point"`
	Price    int64     `field:"price"       validate:"gte=0"`
}

var validate = newValidator()

func newValidator() *validator.Validate {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(f reflect.StructField) string {
		return f.Tag.Get("field")
	})

	return v
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
	err := validate.Struct(p)
	if err == nil {
		return nil
	}

	fieldErrs, ok := errors.AsType[validator.ValidationErrors](err)
	if !ok {
		return fmt.Errorf("validate trip params: %w", err)
	}

	fields := make([]FieldError, 0, len(fieldErrs))
	for _, fe := range fieldErrs {
		_, field, _ := strings.Cut(fe.Namespace(), ".")
		fields = append(fields, FieldError{Field: field, Message: message(fe)})
	}

	return &ValidationError{Fields: fields}
}

func message(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "must not be empty"
	case "gte":
		return "must be >= " + fe.Param()
	case "lte":
		return "must be <= " + fe.Param()
	default:
		return "is invalid"
	}
}
