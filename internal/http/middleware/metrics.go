package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
)

const meterName = "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/middleware"

var durationBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2.5, 5, 7.5, 10}

func Metrics(mp metric.MeterProvider) (func(http.Handler) http.Handler, error) {
	meter := mp.Meter(meterName)

	duration, err := meter.Float64Histogram("http.server.request.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of HTTP server requests"),
		metric.WithExplicitBucketBoundaries(durationBuckets...),
	)
	if err != nil {
		return nil, fmt.Errorf("create http.server.request.duration: %w", err)
	}

	active, err := meter.Int64UpDownCounter("http.server.active_requests",
		metric.WithUnit("{request}"),
		metric.WithDescription("Number of in-flight HTTP server requests"),
	)
	if err != nil {
		return nil, fmt.Errorf("create http.server.active_requests: %w", err)
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attrs := []attribute.KeyValue{
				semconv.HTTPRequestMethodKey.String(r.Method),
				semconv.HTTPRoute(matchRoute(r)),
			}
			inFlight := metric.WithAttributes(attrs...)

			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			started := time.Now()
			completed := false

			active.Add(r.Context(), 1, inFlight)

			defer func() {
				active.Add(r.Context(), -1, inFlight)
				duration.Record(r.Context(), time.Since(started).Seconds(), metric.WithAttributes(
					append(attrs, semconv.HTTPResponseStatusCode(responseStatus(ww, completed)))...,
				))
			}()

			next.ServeHTTP(ww, r)

			completed = true
		})
	}, nil
}

func matchRoute(r *http.Request) string {
	match := chi.NewRouteContext()
	if !chi.RouteContext(r.Context()).Routes.Match(match, r.Method, r.URL.Path) {
		return ""
	}

	return match.RoutePattern()
}
