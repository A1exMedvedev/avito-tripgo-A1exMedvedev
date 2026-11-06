package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	meterName = "github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/infra/postgres"

	operationUnknown = "UNKNOWN"
)

var collectionPattern = regexp.MustCompile(`(?i)\b(?:FROM|INTO|UPDATE)\s+([a-z_][a-z0-9_.]*)`)

type queryStateKey struct{}

type queryState struct {
	started time.Time
	attrs   []attribute.KeyValue
}

type queryTracer struct {
	*otelpgx.Tracer
	duration metric.Float64Histogram
}

func newQueryTracer(tp trace.TracerProvider, mp metric.MeterProvider) (*queryTracer, error) {
	duration, err := mp.Meter(meterName).Float64Histogram("db.client.operation.duration",
		metric.WithUnit("s"),
		metric.WithDescription("Duration of database client operations"),
		metric.WithExplicitBucketBoundaries(0.001, 0.005, 0.01, 0.05, 0.1, 0.5, 1, 5, 10),
	)
	if err != nil {
		return nil, fmt.Errorf("create db.client.operation.duration: %w", err)
	}

	return &queryTracer{
		Tracer: otelpgx.NewTracer(
			otelpgx.WithTracerProvider(tp),
			otelpgx.WithMeterProvider(noop.NewMeterProvider()),
		),
		duration: duration,
	}, nil
}

func (t *queryTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.Tracer.TraceQueryStart(ctx, conn, data)

	operation, collection := parseQuery(data.SQL)
	attrs := []attribute.KeyValue{semconv.DBOperationName(operation)}
	span := trace.SpanFromContext(ctx)

	if collection != "" {
		attrs = append(attrs, semconv.DBCollectionName(collection))

		span.SetName(operation + " " + collection)
		span.SetAttributes(semconv.DBCollectionName(collection))
	}

	return context.WithValue(ctx, queryStateKey{}, queryState{started: time.Now(), attrs: attrs})
}

func (t *queryTracer) TracePrepareStart(ctx context.Context, conn *pgx.Conn, data pgx.TracePrepareStartData) context.Context {
	ctx = t.Tracer.TracePrepareStart(ctx, conn, data)

	if operation, collection := parseQuery(data.SQL); collection != "" {
		span := trace.SpanFromContext(ctx)

		span.SetName("PREPARE " + operation + " " + collection)
		span.SetAttributes(semconv.DBCollectionName(collection))
	}

	return ctx
}

func (t *queryTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	if state, ok := ctx.Value(queryStateKey{}).(queryState); ok {
		attrs := state.attrs

		if errorType := errorTypeOf(data.Err); errorType != "" {
			errorAttr := semconv.ErrorTypeKey.String(errorType)

			attrs = append(attrs, errorAttr)

			trace.SpanFromContext(ctx).SetAttributes(errorAttr)
		}

		t.duration.Record(context.WithoutCancel(ctx), time.Since(state.started).Seconds(), metric.WithAttributes(attrs...))
	}

	t.Tracer.TraceQueryEnd(ctx, conn, data)
}

func parseQuery(query string) (operation, collection string) {
	operation = operationUnknown

	if fields := strings.Fields(query); len(fields) > 0 && !strings.HasPrefix(fields[0], "--") {
		operation = strings.ToUpper(fields[0])
	}

	if match := collectionPattern.FindStringSubmatch(query); match != nil {
		collection = match[1]
	}

	return operation, collection
}

func errorTypeOf(err error) string {
	if err == nil || errors.Is(err, sql.ErrNoRows) || errors.Is(err, pgx.ErrNoRows) {
		return ""
	}

	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code
	}

	return fmt.Sprintf("%T", err)
}
