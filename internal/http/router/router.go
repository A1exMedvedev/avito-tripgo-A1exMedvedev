package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/go-chi/chi/v5"
	nethttpmiddleware "github.com/oapi-codegen/nethttp-middleware"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/domainerror"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/middleware"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/response"
)

const maxBodyBytes = 1 << 20

type Telemetry struct {
	Log            *slog.Logger
	TracerProvider trace.TracerProvider
	MeterProvider  metric.MeterProvider
	Propagator     propagation.TextMapPropagator
}

func New(h Handlers, tel Telemetry) (http.Handler, error) {
	spec, err := api.GetSpec()
	if err != nil {
		return nil, fmt.Errorf("load openapi spec: %w", err)
	}

	spec.Servers = nil

	validator := nethttpmiddleware.OapiRequestValidatorWithOptions(spec, &nethttpmiddleware.Options{
		DoNotValidateServers:  true,
		SilenceServersWarning: true,
		Options: openapi3filter.Options{
			AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
		},
		ErrorHandlerWithOpts: func(_ context.Context, err error, w http.ResponseWriter, r *http.Request, _ nethttpmiddleware.ErrorHandlerOpts) {
			response.Problem(w, r, domainerror.InvalidRequest(validationDetail(err)))
		},
	})

	metrics, err := middleware.Metrics(tel.MeterProvider)
	if err != nil {
		return nil, err
	}

	r := chi.NewRouter()
	r.Use(
		middleware.Recover(tel.Log),
		middleware.Tracing(tel.TracerProvider, tel.Propagator),
		middleware.Logging(tel.Log),
		metrics,
		limitBody(maxBodyBytes),
	)

	return api.HandlerWithOptions(&server{handlers: h}, api.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []api.MiddlewareFunc{validator},
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			response.Problem(w, r, domainerror.InvalidRequest(err.Error()))
		},
	}), nil
}

func validationDetail(err error) string {
	if schemaErr, ok := errors.AsType[*openapi3.SchemaError](err); ok {
		if path := strings.Join(schemaErr.JSONPointer(), "."); path != "" {
			return path + ": " + schemaErr.Reason
		}

		return schemaErr.Reason
	}

	if reqErr, ok := errors.AsType[*openapi3filter.RequestError](err); ok && reqErr.Reason != "" {
		return reqErr.Reason
	}

	return ""
}

func limitBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}
