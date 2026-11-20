package save_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/position/save"
)

type mocks struct {
	trips     *MocktripRepository
	positions *MockpositionRepository
	requests  *MockpositionRequests
	metrics   *Mockmetrics
}

func TestUsecase_Do(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	recordedAt := now.Add(-5 * time.Second)
	point := trip.Point{Latitude: 59.935, Longitude: 30.335}

	in := model.SavePositionInput{TripID: tripID, Point: point, RecordedAt: recordedAt}
	toSave := trip.Position{TripID: tripID, Point: point, RecordedAt: recordedAt}
	saved := trip.Position{ID: 42, TripID: tripID, Point: point, RecordedAt: recordedAt}
	active := trip.Trip{ID: tripID, Status: trip.StatusActive}

	tests := []struct {
		name    string
		setup   func(m mocks)
		in      model.SavePositionInput
		want    trip.Position
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "успех, координата сохранена и опрос поездки отменён",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(active, nil)
				m.trips.EXPECT().UpdateLastPosition(gomock.Any(), tripID, recordedAt).Return(nil)
				m.positions.EXPECT().Save(gomock.Any(), toSave).Return(saved, nil)
				m.requests.EXPECT().Cancel(tripID)
				m.metrics.EXPECT().PositionSaved(gomock.Any())
			},
			in:      in,
			want:    saved,
			wantErr: assert.NoError,
		},
		{
			name:  "ошибка валидации, в репозиторий не ходим",
			setup: func(mocks) {},
			in:    model.SavePositionInput{TripID: tripID, Point: trip.Point{Latitude: 91}, RecordedAt: now.Add(2 * time.Minute)},
			want:  trip.Position{},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...any) bool {
				var validationErr *trip.ValidationError
				return assert.ErrorAs(t, err, &validationErr, msgAndArgs...)
			},
		},
		{
			name: "ошибка, поездка не найдена",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{}, trip.ErrTripNotFound)
			},
			in:      in,
			want:    trip.Position{},
			wantErr: errorIs(trip.ErrTripNotFound),
		},
		{
			name: "ошибка, поездка уже завершена",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{ID: tripID, Status: trip.StatusCompleted}, nil)
			},
			in:      in,
			want:    trip.Position{},
			wantErr: errorIs(trip.ErrTripCompleted),
		},
		{
			name: "ошибка, поездку завершили параллельно",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(active, nil)
				m.trips.EXPECT().UpdateLastPosition(gomock.Any(), tripID, recordedAt).Return(trip.ErrTripCompleted)
			},
			in:      in,
			want:    trip.Position{},
			wantErr: errorIs(trip.ErrTripCompleted),
		},
		{
			name: "ошибка репозитория при сохранении координаты",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(active, nil)
				m.trips.EXPECT().UpdateLastPosition(gomock.Any(), tripID, recordedAt).Return(nil)
				m.positions.EXPECT().Save(gomock.Any(), toSave).Return(trip.Position{}, assert.AnError)
			},
			in:      in,
			want:    trip.Position{},
			wantErr: errorIs(assert.AnError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := mocks{
				trips:     NewMocktripRepository(ctrl),
				positions: NewMockpositionRepository(ctrl),
				requests:  NewMockpositionRequests(ctrl),
				metrics:   NewMockmetrics(ctrl),
			}
			tt.setup(m)

			uc := save.New(m.trips, m.positions, passThroughTx(ctrl), m.requests, m.metrics, func() time.Time { return now })
			got, err := uc.Do(context.Background(), tt.in)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func passThroughTx(ctrl *gomock.Controller) *MocktxManager {
	tx := NewMocktxManager(ctrl)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }).
		AnyTimes()

	return tx
}

func errorIs(target error) assert.ErrorAssertionFunc {
	return func(t assert.TestingT, err error, msgAndArgs ...any) bool {
		return assert.ErrorIs(t, err, target, msgAndArgs...)
	}
}
