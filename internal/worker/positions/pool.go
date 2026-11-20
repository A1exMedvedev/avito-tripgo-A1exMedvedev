package positions

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/worker/positions"

type Config struct {
	CheckInterval time.Duration
	StaleAfter    time.Duration
	Workers       int
	QueueSize     int
	PushTimeout   time.Duration
}

type task struct {
	tripID   uuid.UUID
	driverID uuid.UUID
	seq      uint64
	link     trace.SpanContext
}

type Pool struct {
	cfg     Config
	trips   tripSource
	push    pusher
	clock   clock
	tracer  trace.Tracer
	metrics *poolMetrics
	log     logger

	queue chan task

	mu      sync.Mutex
	pending map[uuid.UUID]uint64
	seq     uint64
	closed  bool

	active atomic.Int64

	scannerCancel context.CancelFunc
	scannerDone   chan struct{}
	workersCancel context.CancelFunc
	workers       sync.WaitGroup
}

func New(
	cfg Config,
	trips tripSource,
	push pusher,
	clk clock,
	tp trace.TracerProvider,
	mp metric.MeterProvider,
	log logger,
) (*Pool, error) {
	p := &Pool{
		cfg:     cfg,
		trips:   trips,
		push:    push,
		clock:   clk,
		tracer:  tp.Tracer(tracerName),
		log:     log,
		queue:   make(chan task, cfg.QueueSize),
		pending: make(map[uuid.UUID]uint64),
	}

	m, err := newPoolMetrics(mp,
		func() int64 { return int64(len(p.queue)) },
		p.active.Load,
	)
	if err != nil {
		return nil, err
	}

	p.metrics = m

	return p, nil
}

func (p *Pool) Start(ctx context.Context) {
	workersCtx, workersCancel := context.WithCancel(context.WithoutCancel(ctx))
	p.workersCancel = workersCancel

	for range p.cfg.Workers {
		p.workers.Go(func() { p.work(workersCtx) })
	}

	scannerCtx, scannerCancel := context.WithCancel(context.WithoutCancel(ctx))
	p.scannerCancel = scannerCancel
	p.scannerDone = make(chan struct{})

	go func() {
		defer close(p.scannerDone)
		p.scan(scannerCtx)
	}()

	p.log.InfoContext(ctx, "position requests started",
		slog.Int("workers", p.cfg.Workers),
		slog.Int("queue_size", p.cfg.QueueSize),
		slog.Duration("check_interval", p.cfg.CheckInterval),
	)
}

func (p *Pool) StopScanner(ctx context.Context) {
	p.scannerCancel()
	<-p.scannerDone

	p.log.InfoContext(ctx, "position scanner stopped")
}

func (p *Pool) StopWorkers(ctx context.Context) error {
	p.mu.Lock()
	p.closed = true
	close(p.queue)
	p.mu.Unlock()

	done := make(chan struct{})

	go func() {
		p.workers.Wait()
		close(done)
	}()

	select {
	case <-done:
		p.workersCancel()
		p.log.InfoContext(ctx, "position workers stopped")

		return nil
	case <-ctx.Done():
		p.workersCancel()
		<-done

		return fmt.Errorf("position workers did not finish the queue in time: %w", ctx.Err())
	}
}

func (p *Pool) Cancel(tripID uuid.UUID) {
	p.mu.Lock()
	delete(p.pending, tripID)
	p.mu.Unlock()
}

func (p *Pool) scan(ctx context.Context) {
	ticks, stop := p.clock.Tick(p.cfg.CheckInterval)
	defer stop()

	p.log.InfoContext(ctx, "position scanner started")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticks:
			p.scanOnce(ctx)
		}
	}
}

func (p *Pool) scanOnce(ctx context.Context) {
	ctx, span := p.tracer.Start(ctx, "positions.scan", trace.WithNewRoot())
	defer span.End()

	trips, err := p.trips.ListStale(ctx, p.cfg.StaleAfter)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list stale trips failed")
		p.log.WarnContext(ctx, "list stale trips failed", slog.Any("error", err))

		return
	}

	span.SetAttributes(attribute.Int("trips.stale", len(trips)))

	for _, t := range trips {
		result := p.enqueue(task{tripID: t.ID, driverID: t.DriverID, link: span.SpanContext()})
		p.metrics.staleCheck(ctx, result)

		if result == resultDropped {
			p.metrics.request(ctx, resultDropped)
			p.log.WarnContext(ctx, "position request dropped: queue is full", slog.String("trip_id", t.ID.String()))
		}
	}
}

func (p *Pool) enqueue(t task) string {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return resultDropped
	}

	if _, ok := p.pending[t.tripID]; ok {
		return resultDuplicate
	}

	p.seq++
	t.seq = p.seq

	select {
	case p.queue <- t:
		p.pending[t.tripID] = t.seq
		return resultQueued
	default:
		return resultDropped
	}
}

func (p *Pool) work(ctx context.Context) {
	for t := range p.queue {
		p.handle(ctx, t)
	}
}

func (p *Pool) handle(ctx context.Context, t task) {
	if !p.isPending(t) {
		return
	}

	defer p.release(t)

	if ctx.Err() != nil {
		return
	}

	p.active.Add(1)
	defer p.active.Add(-1)

	ctx, span := p.tracer.Start(ctx, "positions.request",
		trace.WithNewRoot(),
		trace.WithLinks(trace.Link{SpanContext: t.link}),
		trace.WithAttributes(attribute.String("trip.id", t.tripID.String())),
	)
	defer span.End()

	reqCtx, cancel := context.WithTimeout(ctx, p.cfg.PushTimeout)
	defer cancel()

	started := time.Now()
	err := p.push.RequestPosition(reqCtx, t.driverID, t.tripID)
	if err != nil {
		p.metrics.requestDone(ctx, resultError, time.Since(started))
		span.RecordError(err)
		span.SetStatus(codes.Error, "request position failed")
		p.log.WarnContext(ctx, "request position failed",
			slog.String("trip_id", t.tripID.String()),
			slog.Any("error", err),
		)

		return
	}

	p.metrics.requestDone(ctx, resultSuccess, time.Since(started))
}

func (p *Pool) isPending(t task) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	seq, ok := p.pending[t.tripID]

	return ok && seq == t.seq
}

func (p *Pool) release(t task) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.pending[t.tripID] == t.seq {
		delete(p.pending, t.tripID)
	}
}
