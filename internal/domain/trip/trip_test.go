package trip_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

var (
	tripID     = uuid.MustParse("00000000-0000-4000-8000-000000000001")
	userID     = uuid.MustParse("00000000-0000-4000-8000-000000000002")
	driverID   = uuid.MustParse("00000000-0000-4000-8000-000000000003")
	startedAt  = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	finishedAt = time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)

	emptyUser     = trip.FieldError{Field: "user_id", Message: "must not be empty"}
	negativePrice = trip.FieldError{Field: "price", Message: "must be >= 0"}
)

func validParams(modify ...func(p *trip.Params)) trip.Params {
	p := trip.Params{
		UserID:   userID,
		DriverID: driverID,
		Start:    trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:      trip.Point{Latitude: 59.9290, Longitude: 30.3626},
		Price:    1450,
	}

	for _, m := range modify {
		m(&p)
	}

	return p
}

func errorIsValidation(t assert.TestingT, err error, msgAndArgs ...any) bool {
	var validationErr *trip.ValidationError
	return assert.ErrorAs(t, err, &validationErr, msgAndArgs...)
}

func TestNew(t *testing.T) {
	tests := []struct {
		name    string
		in      trip.Params
		want    trip.Trip
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "успех, поездка создаётся активной",
			in:   validParams(),
			want: trip.Trip{
				ID:        tripID,
				UserID:    userID,
				DriverID:  driverID,
				Start:     trip.Point{Latitude: 59.9398, Longitude: 30.3146},
				End:       trip.Point{Latitude: 59.9290, Longitude: 30.3626},
				Price:     1450,
				Status:    trip.StatusActive,
				StartedAt: startedAt,
			},
			wantErr: assert.NoError,
		},
		{
			name:    "ошибка валидации",
			in:      trip.Params{},
			want:    trip.Trip{},
			wantErr: errorIsValidation,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := trip.New(tripID, tt.in, startedAt)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParams_Validate(t *testing.T) {
	tests := []struct {
		name       string
		in         trip.Params
		wantFields []trip.FieldError
	}{
		{
			name: "успех",
			in:   validParams(),
		},
		{
			name: "нулевая цена допустима",
			in:   validParams(func(p *trip.Params) { p.Price = 0 }),
		},
		{
			name: "границы диапазонов координат допустимы",
			in: validParams(func(p *trip.Params) {
				p.Start = trip.Point{Latitude: 90, Longitude: 180}
				p.End = trip.Point{Latitude: -90, Longitude: -180}
			}),
		},
		{
			name:       "пустой пользователь",
			in:         validParams(func(p *trip.Params) { p.UserID = uuid.Nil }),
			wantFields: []trip.FieldError{emptyUser},
		},
		{
			name:       "пустой водитель",
			in:         validParams(func(p *trip.Params) { p.DriverID = uuid.Nil }),
			wantFields: []trip.FieldError{{Field: "driver_id", Message: "must not be empty"}},
		},
		{
			name:       "отрицательная цена",
			in:         validParams(func(p *trip.Params) { p.Price = -1 }),
			wantFields: []trip.FieldError{negativePrice},
		},
		{
			name:       "широта выше диапазона",
			in:         validParams(func(p *trip.Params) { p.Start.Latitude = 90.1 }),
			wantFields: []trip.FieldError{{Field: "start_point.latitude", Message: "must be <= 90"}},
		},
		{
			name:       "широта ниже диапазона",
			in:         validParams(func(p *trip.Params) { p.End.Latitude = -90.1 }),
			wantFields: []trip.FieldError{{Field: "end_point.latitude", Message: "must be >= -90"}},
		},
		{
			name:       "долгота выше диапазона",
			in:         validParams(func(p *trip.Params) { p.End.Longitude = 180.1 }),
			wantFields: []trip.FieldError{{Field: "end_point.longitude", Message: "must be <= 180"}},
		},
		{
			name:       "долгота ниже диапазона",
			in:         validParams(func(p *trip.Params) { p.Start.Longitude = -180.1 }),
			wantFields: []trip.FieldError{{Field: "start_point.longitude", Message: "must be >= -180"}},
		},
		{
			name: "все ошибки возвращаются вместе",
			in: validParams(func(p *trip.Params) {
				p.UserID = uuid.Nil
				p.Price = -1
			}),
			wantFields: []trip.FieldError{
				emptyUser,
				negativePrice,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.in.Validate()

			if tt.wantFields == nil {
				assert.NoError(t, err)
				return
			}

			var validationErr *trip.ValidationError
			if assert.ErrorAs(t, err, &validationErr) {
				assert.Equal(t, tt.wantFields, validationErr.Fields)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &trip.ValidationError{Fields: []trip.FieldError{
		emptyUser,
		negativePrice,
	}}

	assert.Equal(t, "user_id: must not be empty; price: must be >= 0", err.Error())
}

func TestTrip_Finish(t *testing.T) {
	earlier := startedAt.Add(time.Minute)

	tests := []struct {
		name    string
		in      trip.Trip
		want    trip.Trip
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name:    "успех, активная поездка завершается",
			in:      trip.Trip{ID: tripID, Status: trip.StatusActive},
			want:    trip.Trip{ID: tripID, Status: trip.StatusCompleted, FinishedAt: &finishedAt},
			wantErr: assert.NoError,
		},
		{
			name: "ошибка, поездка уже завершена, finished_at не меняется",
			in:   trip.Trip{ID: tripID, Status: trip.StatusCompleted, FinishedAt: &earlier},
			want: trip.Trip{ID: tripID, Status: trip.StatusCompleted, FinishedAt: &earlier},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...any) bool {
				return assert.ErrorIs(t, err, trip.ErrTripCompleted, msgAndArgs...)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.in

			err := got.Finish(finishedAt)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
