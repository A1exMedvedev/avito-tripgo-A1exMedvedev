package finish_test

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
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/usecase/trip/finish"
)

var (
	now     = time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)
	errRepo = errors.New("connection refused")
)

type deps struct {
	trips   *MocktripRepository
	metrics *Mockmetrics
	usecase *finish.Usecase
}

func newDeps(t *testing.T) deps {
	t.Helper()

	ctrl := gomock.NewController(t)
	d := deps{trips: NewMocktripRepository(ctrl), metrics: NewMockmetrics(ctrl)}

	tx := NewMocktxManager(ctrl)
	tx.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) }).
		AnyTimes()

	d.usecase = finish.New(d.trips, tx, d.metrics, slog.New(slog.DiscardHandler), func() time.Time { return now })

	return d
}

func TestDo_ActiveTrip(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	id := uuid.New()

	gomock.InOrder(
		d.trips.EXPECT().Fetch(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusActive}, nil),
		d.trips.EXPECT().Finish(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, tr trip.Trip) error {
				assert.Equal(t, trip.StatusCompleted, tr.Status)
				assert.Equal(t, &now, tr.FinishedAt)

				return nil
			}),
		d.trips.EXPECT().SaveStatusChange(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, c model.StatusChange) error {
				assert.Equal(t, id, c.TripID)
				assert.Equal(t, new(trip.StatusActive), c.From)
				assert.Equal(t, trip.StatusCompleted, c.To)
				assert.Equal(t, now, c.ChangedAt)
				assert.Equal(t, "trip finished", c.Reason)

				return nil
			}),
		d.metrics.EXPECT().TripCompleted(gomock.Any()),
	)

	got, err := d.usecase.Do(context.Background(), id)

	require.NoError(t, err)
	assert.Equal(t, trip.StatusCompleted, got.Status)
	assert.Equal(t, &now, got.FinishedAt)
}

func TestDo_CompletedTrip_NothingIsWritten(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	id := uuid.New()
	d.trips.EXPECT().Fetch(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusCompleted}, nil)

	_, err := d.usecase.Do(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripCompleted)
}

func TestDo_LostRaceWithConcurrentFinish(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	id := uuid.New()
	d.trips.EXPECT().Fetch(gomock.Any(), id).Return(trip.Trip{ID: id, Status: trip.StatusActive}, nil)
	d.trips.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(trip.ErrTripCompleted)

	_, err := d.usecase.Do(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripCompleted)
}

func TestDo_NotFound(t *testing.T) {
	t.Parallel()

	d := newDeps(t)
	id := uuid.New()
	d.trips.EXPECT().Fetch(gomock.Any(), id).Return(trip.Trip{}, trip.ErrTripNotFound)

	_, err := d.usecase.Do(context.Background(), id)

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestDo_RepositoryErrorsAreReturned(t *testing.T) {
	t.Parallel()

	t.Run("fetch", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.trips.EXPECT().Fetch(gomock.Any(), gomock.Any()).Return(trip.Trip{}, errRepo)

		_, err := d.usecase.Do(context.Background(), uuid.New())

		require.ErrorIs(t, err, errRepo)
	})

	t.Run("finish", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.trips.EXPECT().Fetch(gomock.Any(), gomock.Any()).Return(trip.Trip{Status: trip.StatusActive}, nil)
		d.trips.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(errRepo)

		_, err := d.usecase.Do(context.Background(), uuid.New())

		require.ErrorIs(t, err, errRepo)
	})

	t.Run("status history", func(t *testing.T) {
		t.Parallel()

		d := newDeps(t)
		d.trips.EXPECT().Fetch(gomock.Any(), gomock.Any()).Return(trip.Trip{Status: trip.StatusActive}, nil)
		d.trips.EXPECT().Finish(gomock.Any(), gomock.Any()).Return(nil)
		d.trips.EXPECT().SaveStatusChange(gomock.Any(), gomock.Any()).Return(errRepo)

		_, err := d.usecase.Do(context.Background(), uuid.New())

		require.ErrorIs(t, err, errRepo)
	})
}
