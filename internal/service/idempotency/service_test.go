package idempotency_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/service/idempotency"
)

func TestService_Claim(t *testing.T) {
	const ttl = 24 * time.Hour

	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	key := model.IdempotencyKey{
		Key:         uuid.MustParse("00000000-0000-4000-8000-000000000001"),
		RequestHash: []byte("hash"),
		TripID:      uuid.MustParse("00000000-0000-4000-8000-000000000002"),
		CreatedAt:   now,
	}
	existingTripID := uuid.MustParse("00000000-0000-4000-8000-000000000003")
	expiredBefore := now.Add(-ttl)

	tests := []struct {
		name        string
		setup       func(repo *Mockrepository)
		in          model.IdempotencyKey
		wantTripID  uuid.UUID
		wantClaimed bool
		wantErr     assert.ErrorAssertionFunc
	}{
		{
			name: "успех, ключ занят впервые",
			setup: func(repo *Mockrepository) {
				repo.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, expiredBefore).Return(true, nil)
			},
			in:          key,
			wantTripID:  uuid.Nil,
			wantClaimed: true,
			wantErr:     assert.NoError,
		},
		{
			name: "повтор с тем же телом возвращает уже созданную поездку",
			setup: func(repo *Mockrepository) {
				repo.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, expiredBefore).Return(false, nil)
				repo.EXPECT().FetchIdempotencyKey(gomock.Any(), key.Key).
					Return(model.IdempotencyKey{Key: key.Key, RequestHash: []byte("hash"), TripID: existingTripID}, nil)
			},
			in:          key,
			wantTripID:  existingTripID,
			wantClaimed: false,
			wantErr:     assert.NoError,
		},
		{
			name: "ошибка, тот же ключ с другим телом",
			setup: func(repo *Mockrepository) {
				repo.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, expiredBefore).Return(false, nil)
				repo.EXPECT().FetchIdempotencyKey(gomock.Any(), key.Key).
					Return(model.IdempotencyKey{Key: key.Key, RequestHash: []byte("other"), TripID: existingTripID}, nil)
			},
			in:          key,
			wantTripID:  uuid.Nil,
			wantClaimed: false,
			wantErr:     errorIs(trip.ErrIdempotencyConflict),
		},
		{
			name: "ошибка репозитория при занятии ключа",
			setup: func(repo *Mockrepository) {
				repo.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, expiredBefore).Return(false, assert.AnError)
			},
			in:          key,
			wantTripID:  uuid.Nil,
			wantClaimed: false,
			wantErr:     errorIs(assert.AnError),
		},
		{
			name: "ошибка репозитория при чтении ключа",
			setup: func(repo *Mockrepository) {
				repo.EXPECT().ClaimIdempotencyKey(gomock.Any(), key, expiredBefore).Return(false, nil)
				repo.EXPECT().FetchIdempotencyKey(gomock.Any(), key.Key).Return(model.IdempotencyKey{}, assert.AnError)
			},
			in:          key,
			wantTripID:  uuid.Nil,
			wantClaimed: false,
			wantErr:     errorIs(assert.AnError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			repo := NewMockrepository(ctrl)
			tt.setup(repo)

			gotTripID, gotClaimed, err := idempotency.New(repo, ttl).Claim(context.Background(), tt.in)

			tt.wantErr(t, err)
			assert.Equal(t, tt.wantTripID, gotTripID)
			assert.Equal(t, tt.wantClaimed, gotClaimed)
		})
	}
}

func errorIs(target error) assert.ErrorAssertionFunc {
	return func(t assert.TestingT, err error, msgAndArgs ...any) bool {
		return assert.ErrorIs(t, err, target, msgAndArgs...)
	}
}
