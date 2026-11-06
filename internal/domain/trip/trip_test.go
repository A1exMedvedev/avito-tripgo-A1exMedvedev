package trip_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
)

func validParams() trip.Params {
	return trip.Params{
		UserID:   uuid.New(),
		DriverID: uuid.New(),
		Start:    trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:      trip.Point{Latitude: 59.9290, Longitude: 30.3626},
		Price:    1450,
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	p := validParams()

	got, err := trip.New(id, p, now)

	require.NoError(t, err)
	assert.Equal(t, trip.Trip{
		ID:        id,
		UserID:    p.UserID,
		DriverID:  p.DriverID,
		Start:     p.Start,
		End:       p.End,
		Price:     p.Price,
		Status:    trip.StatusActive,
		StartedAt: now,
	}, got)
}

func TestNew_InvalidParams(t *testing.T) {
	t.Parallel()

	_, err := trip.New(uuid.New(), trip.Params{}, time.Now())

	var validationErr *trip.ValidationError
	require.ErrorAs(t, err, &validationErr)
}

func TestParamsValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(p *trip.Params)
		fields []string
	}{
		{name: "valid", mutate: func(*trip.Params) {}},
		{name: "zero price is allowed", mutate: func(p *trip.Params) { p.Price = 0 }},
		{name: "latitude on the boundary", mutate: func(p *trip.Params) { p.Start.Latitude = 90 }},
		{name: "negative latitude on the boundary", mutate: func(p *trip.Params) { p.Start.Latitude = -90 }},
		{name: "longitude on the boundary", mutate: func(p *trip.Params) { p.End.Longitude = 180 }},
		{name: "negative longitude on the boundary", mutate: func(p *trip.Params) { p.End.Longitude = -180 }},
		{
			name:   "empty user",
			mutate: func(p *trip.Params) { p.UserID = uuid.Nil },
			fields: []string{"user_id"},
		},
		{
			name:   "empty driver",
			mutate: func(p *trip.Params) { p.DriverID = uuid.Nil },
			fields: []string{"driver_id"},
		},
		{
			name:   "negative price",
			mutate: func(p *trip.Params) { p.Price = -1 },
			fields: []string{"price"},
		},
		{
			name:   "latitude above range",
			mutate: func(p *trip.Params) { p.Start.Latitude = 90.1 },
			fields: []string{"start_point.latitude"},
		},
		{
			name:   "latitude below range",
			mutate: func(p *trip.Params) { p.End.Latitude = -90.1 },
			fields: []string{"end_point.latitude"},
		},
		{
			name:   "longitude above range",
			mutate: func(p *trip.Params) { p.End.Longitude = 180.1 },
			fields: []string{"end_point.longitude"},
		},
		{
			name:   "longitude below range",
			mutate: func(p *trip.Params) { p.Start.Longitude = -180.1 },
			fields: []string{"start_point.longitude"},
		},
		{
			name: "all errors are reported together",
			mutate: func(p *trip.Params) {
				p.UserID = uuid.Nil
				p.Price = -1
			},
			fields: []string{"user_id", "price"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := validParams()
			tt.mutate(&p)

			err := p.Validate()

			if tt.fields == nil {
				require.NoError(t, err)
				return
			}

			var validationErr *trip.ValidationError
			require.ErrorAs(t, err, &validationErr)

			fields := make([]string, 0, len(validationErr.Fields))
			for _, f := range validationErr.Fields {
				fields = append(fields, f.Field)
				assert.Contains(t, err.Error(), f.Field+": "+f.Message)
			}

			assert.Equal(t, tt.fields, fields)
		})
	}
}

func TestFinish(t *testing.T) {
	t.Parallel()

	startedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	firstFinish := startedAt.Add(18 * time.Minute)

	tests := []struct {
		name           string
		status         trip.Status
		finishedAt     *time.Time
		wantErr        error
		wantStatus     trip.Status
		wantFinishedAt *time.Time
	}{
		{
			name:           "active trip is completed",
			status:         trip.StatusActive,
			wantStatus:     trip.StatusCompleted,
			wantFinishedAt: &firstFinish,
		},
		{
			name:           "completed trip keeps its finished_at",
			status:         trip.StatusCompleted,
			finishedAt:     new(startedAt),
			wantErr:        trip.ErrTripCompleted,
			wantStatus:     trip.StatusCompleted,
			wantFinishedAt: new(startedAt),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tr := trip.Trip{Status: tt.status, StartedAt: startedAt, FinishedAt: tt.finishedAt}

			err := tr.Finish(firstFinish)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantStatus, tr.Status)
			assert.Equal(t, tt.wantFinishedAt, tr.FinishedAt)
		})
	}
}
