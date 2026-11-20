package positions

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	meterName = "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/worker/positions"

	resultQueued    = "queued"
	resultDuplicate = "duplicate"
	resultDropped   = "dropped"
	resultSuccess   = "success"
	resultError     = "error"
)

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

type poolMetrics struct {
	staleChecks     metric.Int64Counter
	requests        metric.Int64Counter
	requestDuration metric.Float64Histogram
}

func newPoolMetrics(mp metric.MeterProvider, queueSize, activeWorkers func() int64) (*poolMetrics, error) {
	meter := mp.Meter(meterName)

	staleChecks, err := meter.Int64Counter("tripgo.stale_checks",
		metric.WithUnit("{trip}"),
		metric.WithDescription("What the scanner did with a stale trip"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.stale_checks: %w", err)
	}

	requests, err := meter.Int64Counter("tripgo.position_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Outcome of an attempt to request a position"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.position_requests: %w", err)
	}

	requestDuration, err := meter.Float64Histogram("tripgo.position_requests.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of a Push Service call"),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.position_requests.duration: %w", err)
	}

	_, err = meter.Int64ObservableGauge("tripgo.position_requests.queue.size",
		metric.WithUnit("{task}"),
		metric.WithDescription("Number of tasks in the queue"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(queueSize())
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.position_requests.queue.size: %w", err)
	}

	_, err = meter.Int64ObservableGauge("tripgo.position_requests.workers.active",
		metric.WithUnit("{worker}"),
		metric.WithDescription("Number of busy workers"),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(activeWorkers())
			return nil
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.position_requests.workers.active: %w", err)
	}

	return &poolMetrics{staleChecks: staleChecks, requests: requests, requestDuration: requestDuration}, nil
}

func (m *poolMetrics) staleCheck(ctx context.Context, result string) {
	m.staleChecks.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *poolMetrics) request(ctx context.Context, result string) {
	m.requests.Add(ctx, 1, metric.WithAttributes(attribute.String("result", result)))
}

func (m *poolMetrics) requestDone(ctx context.Context, result string, d time.Duration) {
	m.request(ctx, result)
	m.requestDuration.Record(ctx, d.Seconds(), metric.WithAttributes(attribute.String("result", result)))
}
