package middleware

import (
	"net/http"
	"strconv"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/propagation"
	semconv "go.opentelemetry.io/otel/semconv/v1.43.0"
	"go.opentelemetry.io/otel/trace"
)

const errorTypePanic = "panic"

func Tracing(tp trace.TracerProvider, propagator propagation.TextMapPropagator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(annotateSpan(next), "http.server",
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithMeterProvider(noop.NewMeterProvider()),
			otelhttp.WithPropagators(propagator),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string { return spanName(r) }),
		)
	}
}

func annotateSpan(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())

		ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
		completed := false

		defer func() {
			span.SetName(spanName(r))

			if route := routePattern(r); route != "" {
				span.SetAttributes(semconv.HTTPRoute(route))
			}

			status := responseStatus(ww, completed)
			if status < http.StatusInternalServerError {
				return
			}

			errorType := strconv.Itoa(status)
			if !completed {
				errorType = errorTypePanic
			}

			span.SetStatus(codes.Error, http.StatusText(status))
			span.SetAttributes(semconv.ErrorTypeKey.String(errorType), semconv.HTTPResponseStatusCode(status))
		}()

		next.ServeHTTP(ww, r)

		completed = true
	})
}

func spanName(r *http.Request) string {
	if route := routePattern(r); route != "" {
		return r.Method + " " + route
	}

	return r.Method
}
