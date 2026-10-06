package httpapi

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

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
)

const maxBodyBytes = 1 << 20

func NewRouter(h *Handler, log *slog.Logger) (http.Handler, error) {
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
			writeProblem(w, r, problemInvalidRequest, validationDetail(err))
		},
	})

	r := chi.NewRouter()
	r.Use(recoverer(log), limitBody(maxBodyBytes))

	return api.HandlerWithOptions(h, api.ChiServerOptions{
		BaseRouter:  r,
		Middlewares: []api.MiddlewareFunc{validator},
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeProblem(w, r, problemInvalidRequest, err.Error())
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

	return problemInvalidRequest.detail
}

func limitBody(n int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}

				if p == http.ErrAbortHandler {
					panic(p)
				}

				log.ErrorContext(r.Context(), "panic in handler",
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Any("panic", p),
				)
				writeProblem(w, r, problemInternal, "")
			}()

			next.ServeHTTP(w, r)
		})
	}
}
