package telemetry

import (
	"context"
	"fmt"
	"log/slog"

	"go.opentelemetry.io/otel/metric"
)

const meterName = "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/telemetry"

type TripMetrics struct {
	created   metric.Int64Counter
	completed metric.Int64Counter
}

func NewTripMetrics(mp metric.MeterProvider) (*TripMetrics, error) {
	meter := mp.Meter(meterName)

	created, err := meter.Int64Counter("tripgo.trips.created",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of created trips"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.trips.created: %w", err)
	}

	completed, err := meter.Int64Counter("tripgo.trips.completed",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of completed trips"),
	)
	if err != nil {
		return nil, fmt.Errorf("create tripgo.trips.completed: %w", err)
	}

	return &TripMetrics{created: created, completed: completed}, nil
}

func (m *TripMetrics) TripCreated(ctx context.Context) {
	m.created.Add(ctx, 1)
}

func (m *TripMetrics) TripCompleted(ctx context.Context) {
	m.completed.Add(ctx, 1)
}

func RegisterActiveTrips(mp metric.MeterProvider, countActive func(ctx context.Context) (int64, error), log *slog.Logger) error {
	_, err := mp.Meter(meterName).Int64ObservableGauge("tripgo.trips.active",
		metric.WithUnit("{trip}"),
		metric.WithDescription("Number of trips in the active status"),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			active, err := countActive(ctx)
			if err != nil {
				log.WarnContext(ctx, "count active trips failed", slog.Any("error", err))
				return nil
			}

			o.Observe(active)

			return nil
		}),
	)
	if err != nil {
		return fmt.Errorf("create tripgo.trips.active: %w", err)
	}

	return nil
}
