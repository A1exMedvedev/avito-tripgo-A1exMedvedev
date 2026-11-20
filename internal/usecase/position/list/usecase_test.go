package list_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/position/list"
)

type mocks struct {
	trips     *MocktripRepository
	positions *MockpositionRepository
}

func TestUsecase_Do(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	route := []trip.Position{{ID: 1, TripID: tripID}, {ID: 2, TripID: tripID}}

	tests := []struct {
		name    string
		setup   func(m mocks)
		want    []trip.Position
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "успех, маршрут найден",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{ID: tripID}, nil)
				m.positions.EXPECT().List(gomock.Any(), tripID).Return(route, nil)
			},
			want:    route,
			wantErr: assert.NoError,
		},
		{
			name: "ошибка, поездка не найдена",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{}, trip.ErrTripNotFound)
			},
			want:    nil,
			wantErr: errorIs(trip.ErrTripNotFound),
		},
		{
			name: "ошибка репозитория координат",
			setup: func(m mocks) {
				m.trips.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{ID: tripID}, nil)
				m.positions.EXPECT().List(gomock.Any(), tripID).Return(nil, assert.AnError)
			},
			want:    nil,
			wantErr: errorIs(assert.AnError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := mocks{trips: NewMocktripRepository(ctrl), positions: NewMockpositionRepository(ctrl)}
			tt.setup(m)

			got, err := list.New(m.trips, m.positions).Do(context.Background(), tripID)

			tt.wantErr(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func errorIs(target error) assert.ErrorAssertionFunc {
	return func(t assert.TestingT, err error, msgAndArgs ...any) bool {
		return assert.ErrorIs(t, err, target, msgAndArgs...)
	}
}
