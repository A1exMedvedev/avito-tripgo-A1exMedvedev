package get_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/get"
)

func TestUsecase_Do(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	expectedTrip := trip.Trip{ID: tripID, Status: trip.StatusActive}

	tests := []struct {
		name    string
		setup   func(repo *MocktripRepository)
		in      uuid.UUID
		want    trip.Trip
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "успех, поездка найдена",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().Fetch(gomock.Any(), tripID).Return(expectedTrip, nil)
			},
			in:      tripID,
			want:    expectedTrip,
			wantErr: assert.NoError,
		},
		{
			name: "ошибка, поездка не найдена",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{}, trip.ErrTripNotFound)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(trip.ErrTripNotFound),
		},
		{
			name: "ошибка репозитория",
			setup: func(repo *MocktripRepository) {
				repo.EXPECT().Fetch(gomock.Any(), tripID).Return(trip.Trip{}, assert.AnError)
			},
			in:      tripID,
			want:    trip.Trip{},
			wantErr: errorIs(assert.AnError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMocktripRepository(ctrl)
			tt.setup(repo)

			got, err := get.New(repo).Do(context.Background(), tt.in)

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
