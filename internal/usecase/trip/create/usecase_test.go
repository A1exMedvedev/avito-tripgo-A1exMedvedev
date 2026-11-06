package create_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/create"
)

const idempotencyTTL = 24 * time.Hour

var (
	now     = time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	errRepo = errors.New("connection refused")
)

type deps struct {
	trips   *MocktripRepository
	keys    *MockidempotencyRepository
	metrics *Mockmetrics
	usecase *create.Usecase
}

func newDeps(t *testing.T) deps {
	t.Helper()

	ctrl := gomock.NewController(t)
	d := deps{
		trips:   NewMocktripRepository(ctrl),
		keys:    NewMockidempotencyRepository(ctrl),
		metrics: NewMockmetrics(ctrl),
	}

	tx := NewMocktxManager(ctrl)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }).
		AnyTimes()

	d.usecase = create.New(d.trips, d.keys, tx, d.metrics, slog.New(slog.DiscardHandler), func() time.Time { return now }, idempotencyTTL)

	return d
}

func params() trip.Params {
	return trip.Params{
		UserID:   uuid.New(),
		DriverID: uuid.New(),
		Start:    trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:      trip.Point{Latitude: 59.9290, Longitude: 30.3626},
		Price:    1450,
	}
}

func TestDo_Success(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	p := params()

	var saved trip.Trip

	gomock.InOrder(
		d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, tr trip.Trip) error {
				saved = tr
				return nil
			}),
		d.trips.EXPECT().SaveStatusChange(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, c model.StatusChange) error {
				assert.Equal(t, saved.ID, c.TripID)
				assert.Nil(t, c.From)
				assert.Equal(t, trip.StatusActive, c.To)
				assert.Equal(t, now, c.ChangedAt)
				assert.Equal(t, "trip created", c.Reason)

				return nil
			}),
		d.metrics.EXPECT().TripCreated(gomock.Any()),
	)

	got, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: p})

	require.NoError(t, err)
	assert.False(t, got.Replayed)
	assert.Equal(t, saved, got.Trip)
	assert.NotEqual(t, uuid.Nil, got.Trip.ID)
	assert.Equal(t, trip.StatusActive, got.Trip.Status)
	assert.Equal(t, now, got.Trip.StartedAt)
	assert.Equal(t, p.DriverID, got.Trip.DriverID)
}

func TestDo_InvalidParams_RepositoryIsNotCalled(t *testing.T) {
	t.Parallel()

	d := newDeps(t)

	_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: trip.Params{}})

	var validationErr *trip.ValidationError
	require.ErrorAs(t, err, &validationErr)
}

func TestDo_DriverBusy(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(trip.ErrDriverBusy)

	_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: params()})

	require.ErrorIs(t, err, trip.ErrDriverBusy)
}

func TestDo_RepositoryErrorsAreReturned(t *testing.T) {
	t.Parallel()

	t.Run("save", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(errRepo)

		_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: params()})

		require.ErrorIs(t, err, errRepo)
	})

	t.Run("status history", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
		d.trips.EXPECT().SaveStatusChange(gomock.Any(), gomock.Any()).Return(errRepo)

		_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: params()})

		require.ErrorIs(t, err, errRepo)
	})
}

func TestDo_IdempotencyKeyClaimed(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	key := uuid.New()

	gomock.InOrder(
		d.keys.EXPECT().ClaimIdempotencyKey(gomock.Any(), gomock.Any(), now.Add(-idempotencyTTL)).
			DoAndReturn(func(_ context.Context, k model.IdempotencyKey, _ time.Time) (bool, error) {
				assert.Equal(t, key, k.Key)
				assert.Equal(t, now, k.CreatedAt)
				assert.Len(t, k.RequestHash, 32)

				return true, nil
			}),
		d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		d.trips.EXPECT().SaveStatusChange(gomock.Any(), gomock.Any()).Return(nil),
		d.metrics.EXPECT().TripCreated(gomock.Any()),
	)

	got, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: params(), IdempotencyKey: &key})

	require.NoError(t, err)
	assert.False(t, got.Replayed)
}

func recordRequestHash(t *testing.T, p trip.Params) []byte {
	t.Helper()

	d := newDeps(t)
	key := uuid.New()

	var hash []byte

	d.keys.EXPECT().ClaimIdempotencyKey(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, k model.IdempotencyKey, _ time.Time) (bool, error) {
			hash = k.RequestHash
			return true, nil
		})
	d.trips.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil)
	d.trips.EXPECT().SaveStatusChange(gomock.Any(), gomock.Any()).Return(nil)
	d.metrics.EXPECT().TripCreated(gomock.Any())

	_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: p, IdempotencyKey: &key})
	require.NoError(t, err)

	return hash
}

func TestDo_IdempotencyReplay_ReturnsExistingTrip_WithoutMetric(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	key := uuid.New()
	p := params()
	existing := trip.Trip{ID: uuid.New(), Status: trip.StatusCompleted}

	d.keys.EXPECT().ClaimIdempotencyKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	d.keys.EXPECT().FetchIdempotencyKey(gomock.Any(), key).
		Return(model.IdempotencyKey{Key: key, RequestHash: recordRequestHash(t, p), TripID: existing.ID}, nil)
	d.trips.EXPECT().Fetch(gomock.Any(), existing.ID).Return(existing, nil)

	got, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: p, IdempotencyKey: &key})

	require.NoError(t, err)
	assert.True(t, got.Replayed)
	assert.Equal(t, existing, got.Trip)
}

func TestDo_IdempotencyConflict(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	key := uuid.New()

	d.keys.EXPECT().ClaimIdempotencyKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
	d.keys.EXPECT().FetchIdempotencyKey(gomock.Any(), key).
		Return(model.IdempotencyKey{Key: key, RequestHash: recordRequestHash(t, params())}, nil)

	_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: params(), IdempotencyKey: &key})

	require.ErrorIs(t, err, trip.ErrIdempotencyConflict)
}

func TestDo_IdempotencyRepositoryErrors(t *testing.T) {
	t.Parallel()

	t.Run("claim", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		key := uuid.New()
		d.keys.EXPECT().ClaimIdempotencyKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, errRepo)

		_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: params(), IdempotencyKey: &key})

		require.ErrorIs(t, err, errRepo)
	})

	t.Run("fetch", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		key := uuid.New()
		d.keys.EXPECT().ClaimIdempotencyKey(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil)
		d.keys.EXPECT().FetchIdempotencyKey(gomock.Any(), key).Return(model.IdempotencyKey{}, errRepo)

		_, err := d.usecase.Do(context.Background(), model.CreateTripInput{Params: params(), IdempotencyKey: &key})

		require.ErrorIs(t, err, errRepo)
	})
}
