package create_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/create"
)

type mocks struct {
	trips       *MocktripRepository
	idempotency *Mockidempotency
	metrics     *Mockmetrics
	log         *Mocklogger
}

func TestUsecase_Do(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	existingID := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	key := uuid.MustParse("00000000-0000-4000-8000-000000000003")
	now := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)

	params := trip.Params{
		UserID:   uuid.MustParse("00000000-0000-4000-8000-000000000004"),
		DriverID: uuid.MustParse("00000000-0000-4000-8000-000000000005"),
		Start:    trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:      trip.Point{Latitude: 59.9290, Longitude: 30.3626},
		Price:    1450,
	}
	created := trip.Trip{
		ID:        tripID,
		UserID:    params.UserID,
		DriverID:  params.DriverID,
		Start:     params.Start,
		End:       params.End,
		Price:     params.Price,
		Status:    trip.StatusActive,
		StartedAt: now,
	}
	existing := trip.Trip{ID: existingID, Status: trip.StatusCompleted}
	statusChange := model.StatusChange{TripID: tripID, To: trip.StatusActive, Reason: "trip created", ChangedAt: now}
	claimedKey := gomock.Cond(func(k model.IdempotencyKey) bool {
		return k.Key == key && k.TripID == tripID && k.CreatedAt.Equal(now) && len(k.RequestHash) == 32
	})

	tests := []struct {
		name    string
		setup   func(m mocks)
		in      model.CreateTripInput
		want    model.CreateTripResult
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "успех без ключа идемпотентности",
			setup: func(m mocks) {
				m.trips.EXPECT().Save(gomock.Any(), created).Return(nil)
				m.trips.EXPECT().SaveStatusChange(gomock.Any(), statusChange).Return(nil)
				m.metrics.EXPECT().TripCreated(gomock.Any())
				m.log.EXPECT().InfoContext(gomock.Any(), "trip created", gomock.Any())
			},
			in:      model.CreateTripInput{Params: params},
			want:    model.CreateTripResult{Trip: created},
			wantErr: assert.NoError,
		},
		{
			name: "успех, ключ идемпотентности занят впервые",
			setup: func(m mocks) {
				m.idempotency.EXPECT().Claim(gomock.Any(), claimedKey).Return(uuid.Nil, true, nil)
				m.trips.EXPECT().Save(gomock.Any(), created).Return(nil)
				m.trips.EXPECT().SaveStatusChange(gomock.Any(), statusChange).Return(nil)
				m.metrics.EXPECT().TripCreated(gomock.Any())
				m.log.EXPECT().InfoContext(gomock.Any(), "trip created", gomock.Any())
			},
			in:      model.CreateTripInput{Params: params, IdempotencyKey: &key},
			want:    model.CreateTripResult{Trip: created},
			wantErr: assert.NoError,
		},
		{
			name: "повтор с тем же ключом возвращает существующую поездку без метрики и лога",
			setup: func(m mocks) {
				m.idempotency.EXPECT().Claim(gomock.Any(), claimedKey).Return(existingID, false, nil)
				m.trips.EXPECT().Fetch(gomock.Any(), existingID).Return(existing, nil)
			},
			in:      model.CreateTripInput{Params: params, IdempotencyKey: &key},
			want:    model.CreateTripResult{Trip: existing, Replayed: true},
			wantErr: assert.NoError,
		},
		{
			name: "ошибка, ключ уже использован с другим телом",
			setup: func(m mocks) {
				m.idempotency.EXPECT().Claim(gomock.Any(), claimedKey).Return(uuid.Nil, false, trip.ErrIdempotencyConflict)
			},
			in:      model.CreateTripInput{Params: params, IdempotencyKey: &key},
			want:    model.CreateTripResult{},
			wantErr: errorIs(trip.ErrIdempotencyConflict),
		},
		{
			name: "ошибка при занятии ключа",
			setup: func(m mocks) {
				m.idempotency.EXPECT().Claim(gomock.Any(), claimedKey).Return(uuid.Nil, false, assert.AnError)
			},
			in:      model.CreateTripInput{Params: params, IdempotencyKey: &key},
			want:    model.CreateTripResult{},
			wantErr: errorIs(assert.AnError),
		},
		{
			name:  "ошибка валидации, в репозиторий не ходим",
			setup: func(mocks) {},
			in:    model.CreateTripInput{Params: trip.Params{}},
			want:  model.CreateTripResult{},
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...any) bool {
				var validationErr *trip.ValidationError
				return assert.ErrorAs(t, err, &validationErr, msgAndArgs...)
			},
		},
		{
			name: "ошибка, водитель занят",
			setup: func(m mocks) {
				m.trips.EXPECT().Save(gomock.Any(), created).Return(trip.ErrDriverBusy)
			},
			in:      model.CreateTripInput{Params: params},
			want:    model.CreateTripResult{},
			wantErr: errorIs(trip.ErrDriverBusy),
		},
		{
			name: "ошибка репозитория при сохранении поездки",
			setup: func(m mocks) {
				m.trips.EXPECT().Save(gomock.Any(), created).Return(assert.AnError)
			},
			in:      model.CreateTripInput{Params: params},
			want:    model.CreateTripResult{},
			wantErr: errorIs(assert.AnError),
		},
		{
			name: "ошибка репозитория при записи в журнал статусов",
			setup: func(m mocks) {
				m.trips.EXPECT().Save(gomock.Any(), created).Return(nil)
				m.trips.EXPECT().SaveStatusChange(gomock.Any(), statusChange).Return(assert.AnError)
			},
			in:      model.CreateTripInput{Params: params},
			want:    model.CreateTripResult{},
			wantErr: errorIs(assert.AnError),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := mocks{
				trips:       NewMocktripRepository(ctrl),
				idempotency: NewMockidempotency(ctrl),
				metrics:     NewMockmetrics(ctrl),
				log:         NewMocklogger(ctrl),
			}
			tt.setup(m)

			uc := create.New(m.trips, m.idempotency, passThroughTx(ctrl), m.metrics, m.log,
				func() time.Time { return now },
				func() (uuid.UUID, error) { return tripID, nil },
			)
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
