//go:build integration

package trip_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/config"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres/transaction"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
	positionrepo "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/repository/position"
	triprepo "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/repository/trip"
)

const (
	migrationsDir = "../../../migrations"
	queryTimeout  = 5 * time.Second
)

var (
	testPool *pgxpool.Pool
	txm      *transaction.Manager
	repo     *triprepo.Repo
	posRepo  *positionrepo.Repo
)

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("tripgo"),
		tcpostgres.WithUsername("tripgo"),
		tcpostgres.WithPassword("tripgo"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(time.Minute),
		),
	)
	if err != nil {
		log.Printf("start postgres: %v", err)
		return 1
	}

	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			log.Printf("terminate postgres: %v", err)
		}
	}()

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Printf("connection string: %v", err)
		return 1
	}

	if err := migrate(dsn); err != nil {
		log.Printf("migrate: %v", err)
		return 1
	}

	testPool, err = postgres.NewPool(ctx, config.DB{
		URL:             dsn,
		MaxConns:        10,
		MinConns:        1,
		MaxConnLifetime: time.Minute,
		ConnectTimeout:  queryTimeout,
		QueryTimeout:    queryTimeout,
	}, tracenoop.NewTracerProvider(), metricnoop.NewMeterProvider())
	if err != nil {
		log.Printf("create pool: %v", err)
		return 1
	}
	defer testPool.Close()

	txm = transaction.NewManager(testPool, pgx.ReadCommitted, queryTimeout)
	repo = triprepo.New(txm, queryTimeout)
	posRepo = positionrepo.New(txm, queryTimeout)

	return m.Run()
}

func migrate(dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set dialect: %w", err)
	}

	goose.SetLogger(goose.NopLogger())

	return goose.Up(db, migrationsDir)
}

func newTrip(t *testing.T) trip.Trip {
	t.Helper()

	id := uuid.Must(uuid.NewV7())

	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), `DELETE FROM trips WHERE id = $1`, id)
		assert.NoError(t, err)
	})

	return trip.Trip{
		ID:        id,
		UserID:    uuid.New(),
		DriverID:  uuid.New(),
		Start:     trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:       trip.Point{Latitude: 59.9290, Longitude: 30.3626},
		Price:     1450,
		Status:    trip.StatusActive,
		StartedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
}

func countRows(t *testing.T, table string, id uuid.UUID) int {
	t.Helper()

	column := map[string]string{"trips": "id", "trip_status_history": "trip_id", "idempotency_keys": "trip_id"}[table]

	var count int

	err := testPool.QueryRow(context.Background(), fmt.Sprintf(`SELECT count(*) FROM %s WHERE %s = $1`, table, column), id).Scan(&count)
	require.NoError(t, err)

	return count
}

func creation(tr trip.Trip) model.StatusChange {
	return model.StatusChange{TripID: tr.ID, To: trip.StatusActive, Reason: "trip created", ChangedAt: tr.StartedAt}
}

func TestSaveAndFetch(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)

	require.NoError(t, repo.Save(ctx, tr))

	got, err := repo.Fetch(ctx, tr.ID)

	require.NoError(t, err)
	assert.Equal(t, tr, got)
}

func TestFetch_NotFound(t *testing.T) {
	_, err := repo.Fetch(context.Background(), uuid.New())

	require.ErrorIs(t, err, trip.ErrTripNotFound)
}

func TestSave_DriverBusy(t *testing.T) {
	ctx := context.Background()
	first := newTrip(t)
	require.NoError(t, repo.Save(ctx, first))

	second := newTrip(t)
	second.DriverID = first.DriverID

	require.ErrorIs(t, repo.Save(ctx, second), trip.ErrDriverBusy)
}

func TestSave_DriverIsFreeAfterFinish(t *testing.T) {
	ctx := context.Background()
	first := newTrip(t)
	require.NoError(t, repo.Save(ctx, first))
	require.NoError(t, first.Finish(time.Now().UTC().Truncate(time.Microsecond)))
	require.NoError(t, repo.Finish(ctx, first))

	second := newTrip(t)
	second.DriverID = first.DriverID

	require.NoError(t, repo.Save(ctx, second))
}

func TestSave_ConcurrentForOneDriver_OnlyOneWins(t *testing.T) {
	const attempts = 8

	ctx := context.Background()
	driver := uuid.New()

	trips := make([]trip.Trip, attempts)
	for i := range trips {
		trips[i] = newTrip(t)
		trips[i].DriverID = driver
	}

	errs := make([]error, attempts)

	var wg sync.WaitGroup
	for i := range trips {
		wg.Go(func() { errs[i] = repo.Save(ctx, trips[i]) })
	}
	wg.Wait()

	succeeded := 0

	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}

		require.ErrorIs(t, err, trip.ErrDriverBusy)
	}

	assert.Equal(t, 1, succeeded)
}

func TestFinish(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	finishedAt := tr.StartedAt.Add(18 * time.Minute)
	require.NoError(t, tr.Finish(finishedAt))
	require.NoError(t, repo.Finish(ctx, tr))

	got, err := repo.Fetch(ctx, tr.ID)

	require.NoError(t, err)
	assert.Equal(t, trip.StatusCompleted, got.Status)
	assert.Equal(t, &finishedAt, got.FinishedAt)
}

func TestFinish_AlreadyCompleted_KeepsFinishedAt(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	finishedAt := tr.StartedAt.Add(time.Minute)
	first := tr
	require.NoError(t, first.Finish(finishedAt))
	require.NoError(t, repo.Finish(ctx, first))

	second := tr
	require.NoError(t, second.Finish(finishedAt.Add(time.Hour)))
	require.ErrorIs(t, repo.Finish(ctx, second), trip.ErrTripCompleted)

	got, err := repo.Fetch(ctx, tr.ID)

	require.NoError(t, err)
	assert.Equal(t, &finishedAt, got.FinishedAt)
}

func TestFinish_Concurrent_OnlyOneWins(t *testing.T) {
	const attempts = 8

	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	finished := tr
	require.NoError(t, finished.Finish(tr.StartedAt.Add(time.Minute)))

	errs := make([]error, attempts)

	var wg sync.WaitGroup
	for i := range attempts {
		wg.Go(func() { errs[i] = repo.Finish(ctx, finished) })
	}
	wg.Wait()

	succeeded := 0

	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}

		require.ErrorIs(t, err, trip.ErrTripCompleted)
	}

	assert.Equal(t, 1, succeeded)
}

func TestCountActive(t *testing.T) {
	ctx := context.Background()

	before, err := repo.CountActive(ctx)
	require.NoError(t, err)

	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	during, err := repo.CountActive(ctx)
	require.NoError(t, err)
	assert.Equal(t, before+1, during)

	require.NoError(t, tr.Finish(tr.StartedAt.Add(time.Minute)))
	require.NoError(t, repo.Finish(ctx, tr))

	after, err := repo.CountActive(ctx)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestIdempotencyKey_ClaimFetchAndExpire(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	key := model.IdempotencyKey{
		Key:         uuid.New(),
		RequestHash: []byte("hash"),
		TripID:      tr.ID,
		CreatedAt:   tr.StartedAt,
	}
	notExpired := tr.StartedAt.Add(-time.Hour)

	claimed, err := repo.ClaimIdempotencyKey(ctx, key, notExpired)
	require.NoError(t, err)
	assert.True(t, claimed, "первый запрос занимает ключ")

	claimed, err = repo.ClaimIdempotencyKey(ctx, key, notExpired)
	require.NoError(t, err)
	assert.False(t, claimed, "занятый и не просроченный ключ второй раз не занять")

	got, err := repo.FetchIdempotencyKey(ctx, key.Key)
	require.NoError(t, err)
	assert.Equal(t, key.TripID, got.TripID)
	assert.Equal(t, key.RequestHash, got.RequestHash)

	renewed := key
	renewed.RequestHash = []byte("new hash")
	renewed.CreatedAt = tr.StartedAt.Add(time.Hour)

	claimed, err = repo.ClaimIdempotencyKey(ctx, renewed, tr.StartedAt.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, claimed, "просроченный ключ можно занять заново")

	got, err = repo.FetchIdempotencyKey(ctx, key.Key)
	require.NoError(t, err)
	assert.Equal(t, renewed.RequestHash, got.RequestHash)
}

func TestTransaction_CommitWritesBothTables(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)

	err := txm.Do(ctx, func(ctx context.Context) error {
		if err := repo.Save(ctx, tr); err != nil {
			return err
		}

		return repo.SaveStatusChange(ctx, creation(tr))
	})

	require.NoError(t, err)
	assert.Equal(t, 1, countRows(t, "trips", tr.ID))
	assert.Equal(t, 1, countRows(t, "trip_status_history", tr.ID))
}

func TestTransaction_RollbackRevertsBothTables(t *testing.T) {
	ctx := context.Background()

	t.Run("error returned after both inserts", func(t *testing.T) {
		tr := newTrip(t)
		errBoom := errors.New("boom")

		err := txm.Do(ctx, func(ctx context.Context) error {
			if err := repo.Save(ctx, tr); err != nil {
				return err
			}

			if err := repo.SaveStatusChange(ctx, creation(tr)); err != nil {
				return err
			}

			return errBoom
		})

		require.ErrorIs(t, err, errBoom)
		assert.Equal(t, 0, countRows(t, "trips", tr.ID))
		assert.Equal(t, 0, countRows(t, "trip_status_history", tr.ID))
	})

	t.Run("second insert violates a constraint", func(t *testing.T) {
		tr := newTrip(t)
		invalid := creation(tr)
		invalid.To = "unknown"

		err := txm.Do(ctx, func(ctx context.Context) error {
			if err := repo.Save(ctx, tr); err != nil {
				return err
			}

			return repo.SaveStatusChange(ctx, invalid)
		})

		require.Error(t, err)
		assert.Equal(t, 0, countRows(t, "trips", tr.ID))
		assert.Equal(t, 0, countRows(t, "trip_status_history", tr.ID))
	})

	t.Run("panic", func(t *testing.T) {
		tr := newTrip(t)

		assert.Panics(t, func() {
			_ = txm.Do(ctx, func(ctx context.Context) error {
				if err := repo.Save(ctx, tr); err != nil {
					return err
				}

				panic("boom")
			})
		})

		assert.Equal(t, 0, countRows(t, "trips", tr.ID))
	})
}

func TestTransaction_NestedDoSharesOuterTransaction(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	errBoom := errors.New("boom")

	err := txm.Do(ctx, func(ctx context.Context) error {
		if err := txm.Do(ctx, func(ctx context.Context) error { return repo.Save(ctx, tr) }); err != nil {
			return err
		}

		return errBoom
	})

	require.ErrorIs(t, err, errBoom)
	assert.Equal(t, 0, countRows(t, "trips", tr.ID))
}

func staleIDs(t *testing.T, staleAfter time.Duration, among ...uuid.UUID) []uuid.UUID {
	t.Helper()

	stale, err := repo.ListStale(context.Background(), staleAfter)
	require.NoError(t, err)

	wanted := map[uuid.UUID]bool{}
	for _, id := range among {
		wanted[id] = true
	}

	var ids []uuid.UUID

	for _, st := range stale {
		if wanted[st.ID] {
			ids = append(ids, st.ID)
		}
	}

	return ids
}

func TestListStale_SelectsOnlyActiveTripsWithoutFreshPositions(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	silentLongest := newTrip(t)
	silentLongest.StartedAt = now.Add(-3 * time.Hour)

	silent := newTrip(t)
	silent.StartedAt = now.Add(-2 * time.Hour)

	staleByPosition := newTrip(t)
	staleByPosition.StartedAt = now.Add(-5 * time.Hour)

	fresh := newTrip(t)
	fresh.StartedAt = now.Add(-2 * time.Hour)

	justStarted := newTrip(t)
	justStarted.StartedAt = now

	completed := newTrip(t)
	completed.StartedAt = now.Add(-4 * time.Hour)

	for _, tr := range []trip.Trip{silentLongest, silent, staleByPosition, fresh, justStarted, completed} {
		require.NoError(t, repo.Save(ctx, tr))
	}

	require.NoError(t, repo.UpdateLastPosition(ctx, staleByPosition.ID, now.Add(-150*time.Minute)))
	require.NoError(t, repo.UpdateLastPosition(ctx, fresh.ID, now.Add(-time.Minute)))
	require.NoError(t, completed.Finish(now))
	require.NoError(t, repo.Finish(ctx, completed))

	got := staleIDs(t, time.Hour, silentLongest.ID, silent.ID, staleByPosition.ID, fresh.ID, justStarted.ID, completed.ID)

	assert.Equal(t, []uuid.UUID{silentLongest.ID, staleByPosition.ID, silent.ID}, got,
		"без координат отсчёт от started_at, первыми молчащие дольше всех; свежая и завершённая не попадают")
}

func TestUpdateLastPosition_OldPositionDoesNotAgeTrip(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	latest := tr.StartedAt.Add(10 * time.Minute)
	require.NoError(t, repo.UpdateLastPosition(ctx, tr.ID, latest))
	require.NoError(t, repo.UpdateLastPosition(ctx, tr.ID, latest.Add(-5*time.Minute)))

	got, err := repo.Fetch(ctx, tr.ID)
	require.NoError(t, err)
	assert.Equal(t, &latest, got.LastPositionAt)
}

func TestUpdateLastPosition_CompletedTrip(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))
	require.NoError(t, tr.Finish(tr.StartedAt.Add(time.Minute)))
	require.NoError(t, repo.Finish(ctx, tr))

	require.ErrorIs(t, repo.UpdateLastPosition(ctx, tr.ID, tr.StartedAt), trip.ErrTripCompleted)
}

func savePosition(ctx context.Context, p trip.Position) error {
	return txm.Do(ctx, func(ctx context.Context) error {
		if err := repo.UpdateLastPosition(ctx, p.TripID, p.RecordedAt); err != nil {
			return err
		}

		_, err := posRepo.Save(ctx, p)

		return err
	})
}

func TestSavePosition_UpdatesLastPositionAt(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	recordedAt := tr.StartedAt.Add(time.Minute)
	require.NoError(t, savePosition(ctx, trip.Position{TripID: tr.ID, Point: trip.Point{Latitude: 59.93, Longitude: 30.33}, RecordedAt: recordedAt}))

	got, err := repo.Fetch(ctx, tr.ID)
	require.NoError(t, err)
	assert.Equal(t, &recordedAt, got.LastPositionAt)

	positions, err := posRepo.List(ctx, tr.ID)
	require.NoError(t, err)
	require.Len(t, positions, 1)
	assert.Equal(t, recordedAt, positions[0].RecordedAt)
}

func TestSavePosition_RollsBackAtomically(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	err := savePosition(ctx, trip.Position{TripID: tr.ID, Point: trip.Point{Latitude: 100}, RecordedAt: tr.StartedAt.Add(time.Minute)})

	require.Error(t, err, "широта вне диапазона нарушает CHECK, вставка падает после обновления trips")

	got, err := repo.Fetch(ctx, tr.ID)
	require.NoError(t, err)
	assert.Nil(t, got.LastPositionAt, "обновление last_position_at откатилось вместе со вставкой")

	positions, err := posRepo.List(ctx, tr.ID)
	require.NoError(t, err)
	assert.Empty(t, positions)
}

func TestListPositions_SortedByRecordedAtThenID(t *testing.T) {
	ctx := context.Background()
	tr := newTrip(t)
	require.NoError(t, repo.Save(ctx, tr))

	later := tr.StartedAt.Add(2 * time.Minute)
	earlier := tr.StartedAt.Add(time.Minute)

	recorded := []time.Time{later, earlier, earlier}
	saved := make([]trip.Position, 0, len(recorded))

	for _, at := range recorded {
		p, err := posRepo.Save(ctx, trip.Position{TripID: tr.ID, Point: trip.Point{Latitude: 1, Longitude: 1}, RecordedAt: at})
		require.NoError(t, err)

		saved = append(saved, p)
	}

	got, err := posRepo.List(ctx, tr.ID)
	require.NoError(t, err)
	assert.Equal(t, []trip.Position{saved[1], saved[2], saved[0]}, got)
}
