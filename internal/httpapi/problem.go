package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
)

type problem struct {
	status int
	code   string
	slug   string
	title  string
	detail string
}

var (
	problemInvalidRequest      = problem{http.StatusBadRequest, "invalid_request", "invalid-request", "Invalid request", "Request validation failed"}
	problemTripNotFound        = problem{http.StatusNotFound, "trip_not_found", "trip-not-found", "Trip not found", "Trip was not found"}
	problemTripCompleted       = problem{http.StatusConflict, "trip_completed", "trip-completed", "Trip completed", "Operation is not allowed for a completed trip"}
	problemDriverBusy          = problem{http.StatusConflict, "driver_busy", "driver-busy", "Driver busy", "Driver already has an active trip"}
	problemIdempotencyConflict = problem{http.StatusConflict, "idempotency_conflict", "idempotency-conflict", "Idempotency conflict", "Idempotency-Key was already used with a different request body"}
	problemInternal            = problem{http.StatusInternalServerError, "internal_error", "internal-error", "Internal Server Error", "Internal server error"}
)

func writeProblem(w http.ResponseWriter, r *http.Request, p problem, detail string) {
	if detail == "" {
		detail = p.detail
	}

	body := api.Problem{
		Type:     "https://tripgo.example/problems/" + p.slug,
		Title:    p.title,
		Status:   int32(p.status),
		Detail:   &detail,
		Instance: new(r.URL.Path),
		Code:     p.code,
	}

	writeBody(w, "application/problem+json", p.status, body)
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	writeBody(w, "application/json", status, body)
}

func writeBody(w http.ResponseWriter, contentType string, status int, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
