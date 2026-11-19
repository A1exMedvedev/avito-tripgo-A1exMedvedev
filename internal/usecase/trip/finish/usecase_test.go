package finish_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/finish"
)

type mocks struct {
	trips   *MocktripRepository
	metrics *Mockmetrics
	log     *Mocklogger
}

func TestUsecase_Do(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	now := time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)

	active := trip.Trip{ID: tripID, Status: trip.StatusActive}
	completed := trip.Trip{ID: tripID, Status: trip.StatusCompleted, FinishedAt: &now}
	statusChange := model.StatusChange{
		TripID:    tripID,
		From:      new(trip.StatusActive),
		To:        trip.StatusCompleted,
		Reason:    "trip finished",
		ChangedAt: now,
	}

	tests := []struct {
		name    string
		setup   func(m mocks)
		in      uuid.UUID
		want    trip.Trip
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "успех, активная поездка завершена",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(active, nil)
				m.trips.EXPECT().Finish(gomock.Any(), completed).Return(nil)
				m.trips.EXPECT().SaveStatusChange(gomock.Any(), statusChange).Return(nil)
				m.metrics.EXPECT().TripCompleted(gomock.Any())
				m.log.EXPECT().InfoContext(gomock.Any(), "trip completed", gomock.Any())
			},
			in:      tripID,
			want:    completed,
			wantErr: assert.NoError,
		},
		{
			name: "ошибка, поездка уже завершена, в бд ничего не пишем",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(completed, nil)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(trip.ErrTripCompleted),
		},
		{
			name: "ошибка, параллельный запрос успел завершить поездку",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(active, nil)
				m.trips.EXPECT().Finish(gomock.Any(), completed).Return(trip.ErrTripCompleted)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(trip.ErrTripCompleted),
		},
		{
			name: "ошибка, поездка не найдена",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{}, trip.ErrTripNotFound)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(trip.ErrTripNotFound),
		},
		{
			name: "ошибка репозитория при чтении",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{}, assert.AnError)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(assert.AnError),
		},
		{
			name: "ошибка репозитория при обновлении",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(active, nil)
				m.trips.EXPECT().Finish(gomock.Any(), completed).Return(assert.AnError)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(assert.AnError),
		},
		{
			name: "ошибка репозитория при записи в журнал статусов",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(active, nil)
				m.trips.EXPECT().Finish(gomock.Any(), completed).Return(nil)
				m.trips.EXPECT().SaveStatusChange(gomock.Any(), statusChange).Return(assert.AnError)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(assert.AnError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := mocks{
				trips:   NewMocktripRepository(ctrl),
				metrics: NewMockmetrics(ctrl),
				log:     NewMocklogger(ctrl),
			}
			tt.setup(m)

			uc := finish.New(m.trips, passThroughTx(ctrl), m.metrics, m.log, func() time.Time { return now })
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
