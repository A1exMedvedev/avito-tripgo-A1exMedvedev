package domainerror

import (
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/trace"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/response"
)

const problemTypePrefix = "https://tripgo.example/problems/"

type kind struct {
	status int32
	code   string
	slug   string
	title  string
	detail string
}

var (
	invalidRequest      = kind{http.StatusBadRequest, "invalid_request", "invalid-request", "Invalid request", "Request validation failed"}
	tripNotFound        = kind{http.StatusNotFound, "trip_not_found", "trip-not-found", "Trip not found", "Trip was not found"}
	tripCompleted       = kind{http.StatusConflict, "trip_completed", "trip-completed", "Trip completed", "Operation is not allowed for a completed trip"}
	driverBusy          = kind{http.StatusConflict, "driver_busy", "driver-busy", "Driver busy", "Driver already has an active trip"}
	idempotencyConflict = kind{http.StatusConflict, "idempotency_conflict", "idempotency-conflict", "Idempotency conflict", "Idempotency-Key was already used with a different request body"}
	internal            = kind{http.StatusInternalServerError, "internal_error", "internal-error", "Internal Server Error", "Internal server error"}
)

func Problem(err error) (status int, body api.Problem) {
	if validationErr, ok := errors.AsType[*trip.ValidationError](err); ok {
		return int(invalidRequest.status), invalidRequest.problem(validationErr.Error())
	}

	k := internal

	switch {
	case errors.Is(err, trip.ErrTripNotFound):
		k = tripNotFound
	case errors.Is(err, trip.ErrTripCompleted):
		k = tripCompleted
	case errors.Is(err, trip.ErrDriverBusy):
		k = driverBusy
	case errors.Is(err, trip.ErrIdempotencyConflict):
		k = idempotencyConflict
	}

	return int(k.status), k.problem(k.detail)
}

func InvalidRequest(detail string) api.Problem {
	if detail == "" {
		detail = invalidRequest.detail
	}

	return invalidRequest.problem(detail)
}

func Write(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	status, body := Problem(err)

	if status >= http.StatusInternalServerError {
		log.ErrorContext(r.Context(), "request failed", slog.Any("error", err))
		trace.SpanFromContext(r.Context()).RecordError(err)
	}

	response.Problem(w, r, body)
}

func (k kind) problem(detail string) api.Problem {
	return api.Problem{
		Type:   problemTypePrefix + k.slug,
		Title:  k.title,
		Status: k.status,
		Detail: &detail,
		Code:   k.code,
	}
}
