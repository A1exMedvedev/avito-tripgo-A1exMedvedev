package positions

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

type fakeClock struct {
	ticks chan time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{ticks: make(chan time.Time)}
}

func (c *fakeClock) Tick(time.Duration) (ticks <-chan time.Time, stop func()) {
	return c.ticks, func() {}
}

type fixture struct {
	pool   *Pool
	trips  *MocktripSource
	push   *Mockpusher
	log    *Mocklogger
	clock  *fakeClock
	reader *sdkmetric.ManualReader
}

func newFixture(t *testing.T, workers, queueSize int) *fixture {
	t.Helper()

	ctrl := gomock.NewController(t)
	f := &fixture{
		trips:  NewMocktripSource(ctrl),
		push:   NewMockpusher(ctrl),
		log:    NewMocklogger(ctrl),
		clock:  newFakeClock(),
		reader: sdkmetric.NewManualReader(),
	}

	f.log.EXPECT().InfoContext(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes()

	pool, err := New(
		Config{
			CheckInterval: 30 * time.Second,
			StaleAfter:    time.Minute,
			Workers:       workers,
			QueueSize:     queueSize,
			PushTimeout:   time.Second,
		},
		f.trips, f.push, f.clock,
		tracenoop.NewTracerProvider(),
		sdkmetric.NewMeterProvider(sdkmetric.WithReader(f.reader)),
		f.log,
	)
	require.NoError(t, err)

	f.pool = pool

	return f
}

func (f *fixture) stop(t *testing.T) {
	t.Helper()

	f.pool.StopScanner(context.Background())
	require.NoError(t, f.pool.StopWorkers(context.Background()))
}

func (f *fixture) counters(t *testing.T, name string) map[string]int64 {
	t.Helper()

	result := map[string]int64{}

	for _, point := range f.collect(t, name, func(m metricdata.Metrics) []metricdata.DataPoint[int64] {
		if sum, ok := m.Data.(metricdata.Sum[int64]); ok {
			return sum.DataPoints
		}

		return nil
	}) {
		value, _ := point.Attributes.Value(attribute.Key("result"))
		result[value.AsString()] = point.Value
	}

	return result
}

func (f *fixture) gauge(t *testing.T, name string) int64 {
	t.Helper()

	points := f.collect(t, name, func(m metricdata.Metrics) []metricdata.DataPoint[int64] {
		if g, ok := m.Data.(metricdata.Gauge[int64]); ok {
			return g.DataPoints
		}

		return nil
	})
	require.Len(t, points, 1)

	return points[0].Value
}

func (f *fixture) collect(t *testing.T, name string, points func(metricdata.Metrics) []metricdata.DataPoint[int64]) []metricdata.DataPoint[int64] {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, f.reader.Collect(context.Background(), &rm))

	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == name {
				return points(m)
			}
		}
	}

	return nil
}

func staleTrips(n int) []model.StaleTrip {
	trips := make([]model.StaleTrip, n)
	for i := range trips {
		trips[i] = model.StaleTrip{ID: uuid.New(), DriverID: uuid.New()}
	}

	return trips
}

func TestPool_ScanOnce(t *testing.T) {
	trips := staleTrips(3)

	tests := []struct {
		name          string
		queueSize     int
		setup         func(f *fixture)
		wantChecks    map[string]int64
		wantRequests  map[string]int64
		wantQueueSize int64
	}{
		{
			name:      "все устаревшие поездки встают в очередь",
			queueSize: 10,
			setup: func(f *fixture) {
				f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trips, nil)
			},
			wantChecks:    map[string]int64{resultQueued: 3},
			wantRequests:  map[string]int64{},
			wantQueueSize: 3,
		},
		{
			name:      "поездка, уже стоящая в очереди, повторным тиком не добавляется",
			queueSize: 10,
			setup: func(f *fixture) {
				f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trips[:1], nil)
				f.pool.scanOnce(context.Background())
				f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trips[:1], nil)
			},
			wantChecks:    map[string]int64{resultQueued: 1, resultDuplicate: 1},
			wantRequests:  map[string]int64{},
			wantQueueSize: 1,
		},
		{
			name:      "переполнение очереди даёт dropped, а не блокировку сканера",
			queueSize: 1,
			setup: func(f *fixture) {
				f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trips, nil)
				f.log.EXPECT().WarnContext(gomock.Any(), "position request dropped: queue is full", gomock.Any()).Times(2)
			},
			wantChecks:    map[string]int64{resultQueued: 1, resultDropped: 2},
			wantRequests:  map[string]int64{resultDropped: 2},
			wantQueueSize: 1,
		},
		{
			name:      "ошибка выборки логируется, в очередь ничего не попадает",
			queueSize: 10,
			setup: func(f *fixture) {
				f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(nil, assert.AnError)
				f.log.EXPECT().WarnContext(gomock.Any(), "list stale trips failed", gomock.Any())
			},
			wantChecks:    map[string]int64{},
			wantRequests:  map[string]int64{},
			wantQueueSize: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, 0, tt.queueSize)
			tt.setup(f)

			f.pool.scanOnce(context.Background())

			assert.Equal(t, tt.wantChecks, f.counters(t, "tripgo.stale_checks"))
			assert.Equal(t, tt.wantRequests, f.counters(t, "tripgo.position_requests"))
			assert.Equal(t, tt.wantQueueSize, f.gauge(t, "tripgo.position_requests.queue.size"))
		})
	}
}

func TestPool_ScannerRunsOnTickAndTicksDoNotOverlap(t *testing.T) {
	f := newFixture(t, 0, 10)

	scanning := make(chan struct{})
	release := make(chan struct{})

	f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).DoAndReturn(func(context.Context, time.Duration) ([]model.StaleTrip, error) {
		scanning <- struct{}{}
		<-release

		return nil, nil
	}).Times(2)

	f.pool.Start(context.Background())

	f.clock.ticks <- time.Now()
	<-scanning

	select {
	case f.clock.ticks <- time.Now():
		t.Fatal("сканер принял следующий тик, не закончив обход")
	default:
	}

	release <- struct{}{}

	f.clock.ticks <- time.Now()
	<-scanning
	release <- struct{}{}

	f.stop(t)
}

func TestPool_PositionBetweenQueueAndProcessingCancelsPush(t *testing.T) {
	f := newFixture(t, 1, 10)
	trips := staleTrips(2)
	cancelled, kept := trips[0], trips[1]
	done := make(chan struct{})

	f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trips, nil)
	f.push.EXPECT().RequestPosition(gomock.Any(), kept.DriverID, kept.ID).DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
		close(done)
		return nil
	})

	f.pool.scanOnce(context.Background())
	f.pool.Cancel(cancelled.ID)
	f.pool.Start(context.Background())

	<-done

	f.stop(t)

	assert.Equal(t, map[string]int64{resultSuccess: 1}, f.counters(t, "tripgo.position_requests"))
}

func TestPool_CancelledTripCanBeQueuedAgain(t *testing.T) {
	f := newFixture(t, 0, 10)
	trip := staleTrips(1)

	f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trip, nil).Times(2)

	f.pool.scanOnce(context.Background())
	f.pool.Cancel(trip[0].ID)
	f.pool.scanOnce(context.Background())

	assert.Equal(t, map[string]int64{resultQueued: 2}, f.counters(t, "tripgo.stale_checks"))
}

func TestPool_ProcessesTasksWithFixedNumberOfWorkers(t *testing.T) {
	const (
		workers = 3
		tasks   = 9
	)

	f := newFixture(t, workers, tasks)

	var (
		active    atomic.Int64
		maxActive atomic.Int64
		processed sync.WaitGroup
	)

	entered := make(chan struct{}, tasks)
	release := make(chan struct{})

	processed.Add(tasks)
	f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(staleTrips(tasks), nil)
	f.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
		defer processed.Done()

		n := active.Add(1)
		for {
			current := maxActive.Load()
			if n <= current || maxActive.CompareAndSwap(current, n) {
				break
			}
		}

		entered <- struct{}{}
		<-release
		active.Add(-1)

		return nil
	}).Times(tasks)

	f.pool.scanOnce(context.Background())
	f.pool.Start(context.Background())

	for range workers {
		<-entered
	}

	assert.Equal(t, int64(workers), f.gauge(t, "tripgo.position_requests.workers.active"))

	close(release)
	processed.Wait()
	f.stop(t)

	assert.Equal(t, int64(workers), maxActive.Load())
	assert.Equal(t, map[string]int64{resultSuccess: tasks}, f.counters(t, "tripgo.position_requests"))
}

func TestPool_PushErrorDoesNotStopWorker(t *testing.T) {
	f := newFixture(t, 1, 10)
	trips := staleTrips(2)
	done := make(chan struct{})

	f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trips, nil)
	gomock.InOrder(
		f.push.EXPECT().RequestPosition(gomock.Any(), trips[0].DriverID, trips[0].ID).Return(assert.AnError),
		f.push.EXPECT().RequestPosition(gomock.Any(), trips[1].DriverID, trips[1].ID).DoAndReturn(func(context.Context, uuid.UUID, uuid.UUID) error {
			close(done)
			return nil
		}),
	)
	f.log.EXPECT().WarnContext(gomock.Any(), "request position failed", gomock.Any())

	f.pool.scanOnce(context.Background())
	f.pool.Start(context.Background())

	<-done

	f.stop(t)

	assert.Equal(t, map[string]int64{resultError: 1, resultSuccess: 1}, f.counters(t, "tripgo.position_requests"))
}

func TestPool_StopFinishesQueuedTasks(t *testing.T) {
	f := newFixture(t, 2, 10)
	trips := staleTrips(5)

	f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(trips, nil)
	f.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(len(trips))

	f.pool.scanOnce(context.Background())
	f.pool.Start(context.Background())
	f.stop(t)

	assert.Equal(t, map[string]int64{resultSuccess: int64(len(trips))}, f.counters(t, "tripgo.position_requests"))
}

func TestPool_StopWorkersCancelsPushWhenTimeIsOver(t *testing.T) {
	f := newFixture(t, 1, 10)
	entered := make(chan struct{})

	f.trips.EXPECT().ListStale(gomock.Any(), time.Minute).Return(staleTrips(1), nil)
	f.push.EXPECT().RequestPosition(gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, _, _ uuid.UUID) error {
		close(entered)
		<-ctx.Done()

		return ctx.Err()
	})
	f.log.EXPECT().WarnContext(gomock.Any(), "request position failed", gomock.Any())

	f.pool.scanOnce(context.Background())
	f.pool.Start(context.Background())
	<-entered

	f.pool.StopScanner(context.Background())

	expired, cancel := context.WithCancel(context.Background())
	cancel()

	err := f.pool.StopWorkers(expired)

	assert.True(t, errors.Is(err, context.Canceled))
}

func TestPool_ScannerStopsOnCancel(t *testing.T) {
	f := newFixture(t, 2, 10)

	f.pool.Start(context.Background())
	f.stop(t)

	assert.Equal(t, int64(0), f.gauge(t, "tripgo.position_requests.workers.active"))
}

var _ logger = slog.New(slog.DiscardHandler)
