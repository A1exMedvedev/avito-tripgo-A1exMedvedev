package domainerror_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/domainerror"
)

func TestProblem(t *testing.T) {
	t.Parallel()

	validationErr := &trip.ValidationError{Fields: []trip.FieldError{{Field: "price", Message: "must not be negative"}}}

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
		wantType   string
		wantDetail string
	}{
		{
			name:       "validation",
			err:        validationErr,
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantType:   "https://tripgo.example/problems/invalid-request",
			wantDetail: "price: must not be negative",
		},
		{
			name:       "not found",
			err:        trip.ErrTripNotFound,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
			wantType:   "https://tripgo.example/problems/trip-not-found",
			wantDetail: "Trip was not found",
		},
		{
			name:       "completed",
			err:        trip.ErrTripCompleted,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
			wantType:   "https://tripgo.example/problems/trip-completed",
			wantDetail: "Operation is not allowed for a completed trip",
		},
		{
			name:       "driver busy",
			err:        trip.ErrDriverBusy,
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
			wantType:   "https://tripgo.example/problems/driver-busy",
			wantDetail: "Driver already has an active trip",
		},
		{
			name:       "idempotency conflict",
			err:        trip.ErrIdempotencyConflict,
			wantStatus: http.StatusConflict,
			wantCode:   "idempotency_conflict",
			wantType:   "https://tripgo.example/problems/idempotency-conflict",
			wantDetail: "Idempotency-Key was already used with a different request body",
		},
		{
			name:       "wrapped domain error",
			err:        fmt.Errorf("finish trip: %w", trip.ErrTripCompleted),
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
			wantType:   "https://tripgo.example/problems/trip-completed",
			wantDetail: "Operation is not allowed for a completed trip",
		},
		{
			name:       "wrapped validation error",
			err:        fmt.Errorf("create trip: %w", validationErr),
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
			wantType:   "https://tripgo.example/problems/invalid-request",
			wantDetail: "price: must not be negative",
		},
		{
			name:       "unknown error hides details",
			err:        errors.New(`insert trip: ERROR: relation "trips" does not exist`),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
			wantType:   "https://tripgo.example/problems/internal-error",
			wantDetail: "Internal server error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			status, body := domainerror.Problem(tt.err)

			assert.Equal(t, tt.wantStatus, status)
			assert.Equal(t, tt.wantStatus, int(body.Status))
			assert.Equal(t, tt.wantCode, body.Code)
			assert.Equal(t, tt.wantType, body.Type)
			assert.NotEmpty(t, body.Title)
			require.NotNil(t, body.Detail)
			assert.Equal(t, tt.wantDetail, *body.Detail)
		})
	}
}

func TestInvalidRequest(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "price: too small", *domainerror.InvalidRequest("price: too small").Detail)
	assert.Equal(t, "Request validation failed", *domainerror.InvalidRequest("").Detail)
}

func TestWrite(t *testing.T) {
	t.Parallel()

	const secret = "password authentication failed for user tripgo"

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantLogged bool
	}{
		{name: "client error is not logged", err: trip.ErrTripNotFound, wantStatus: http.StatusNotFound},
		{name: "internal error goes to the log only", err: errors.New(secret), wantStatus: http.StatusInternalServerError, wantLogged: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var logs bytes.Buffer

			log := slog.New(slog.NewJSONHandler(&logs, nil))
			rec := httptest.NewRecorder()

			domainerror.Write(rec, httptest.NewRequest(http.MethodGet, "/api/v1/trips/42", nil), log, tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
			assert.NotContains(t, rec.Body.String(), secret)

			var body api.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "/api/v1/trips/42", *body.Instance)

			if tt.wantLogged {
				assert.Contains(t, logs.String(), secret)
			} else {
				assert.Empty(t, logs.String())
			}
		})
	}
}
