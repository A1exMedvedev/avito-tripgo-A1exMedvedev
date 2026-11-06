package get_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/get"
)

func TestDo(t *testing.T) {
	t.Parallel()

	errRepo := errors.New("connection refused")
	found := trip.Trip{ID: uuid.New(), Status: trip.StatusActive}

	tests := []struct {
		name    string
		repoTr  trip.Trip
		repoErr error
		want    trip.Trip
		wantErr error
	}{
		{name: "found", repoTr: found, want: found},
		{name: "not found", repoErr: trip.ErrTripNotFound, wantErr: trip.ErrTripNotFound},
		{name: "repository error", repoErr: errRepo, wantErr: errRepo},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			id := uuid.New()
			repo := NewMocktripRepository(gomock.NewController(t))
			repo.EXPECT().Fetch(gomock.Any(), id).Return(tt.repoTr, tt.repoErr)

			got, err := get.New(repo).Do(context.Background(), id)

			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.want, got)
		})
	}
}
