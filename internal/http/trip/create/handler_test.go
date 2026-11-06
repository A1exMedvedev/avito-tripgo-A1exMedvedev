package create_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/create"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

const validBody = `{
	"user_id": "5cb72c04-7650-45c9-a79b-bcdba0631e0c",
	"driver_id": "8860b315-ec86-42eb-a17c-7c163d721ff5",
	"start_point": {"latitude": 59.9398, "longitude": 30.3146},
	"end_point": {"latitude": 59.9290, "longitude": 30.3626},
	"price": 1450
}`

func perform(t *testing.T, mock *Mockusecase, body string, params api.CreateTripParams) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(body))

	create.New(mock, slog.New(slog.DiscardHandler)).Handle(rec, req, params)

	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) api.Problem {
	t.Helper()

	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))

	return problem
}

func TestHandle_Created(t *testing.T) {
	t.Parallel()

	created := trip.Trip{
		ID:        uuid.New(),
		Status:    trip.StatusActive,
		StartedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
	}

	mock := NewMockusecase(gomock.NewController(t))
	mock.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in model.CreateTripInput) (model.CreateTripResult, error) {
			assert.Equal(t, uuid.MustParse("5cb72c04-7650-45c9-a79b-bcdba0631e0c"), in.Params.UserID)
			assert.Equal(t, uuid.MustParse("8860b315-ec86-42eb-a17c-7c163d721ff5"), in.Params.DriverID)
			assert.Equal(t, trip.Point{Latitude: 59.9398, Longitude: 30.3146}, in.Params.Start)
			assert.Equal(t, trip.Point{Latitude: 59.9290, Longitude: 30.3626}, in.Params.End)
			assert.Equal(t, int64(1450), in.Params.Price)
			assert.Nil(t, in.IdempotencyKey)

			return model.CreateTripResult{Trip: created}, nil
		})

	rec := perform(t, mock, validBody, api.CreateTripParams{})

	assert.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "/api/v1/trips/"+created.ID.String(), rec.Header().Get("Location"))
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, created.ID.String(), got["id"])
	assert.Equal(t, "active", got["status"])
	assert.Contains(t, got, "finished_at")
	assert.Nil(t, got["finished_at"])
}

func TestHandle_Replayed_ReturnsOK(t *testing.T) {
	t.Parallel()

	key := uuid.New()
	existing := trip.Trip{ID: uuid.New(), Status: trip.StatusActive}

	mock := NewMockusecase(gomock.NewController(t))
	mock.EXPECT().Do(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in model.CreateTripInput) (model.CreateTripResult, error) {
			assert.Equal(t, &key, in.IdempotencyKey)
			return model.CreateTripResult{Trip: existing, Replayed: true}, nil
		})

	rec := perform(t, mock, validBody, api.CreateTripParams{IdempotencyKey: &key})

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "/api/v1/trips/"+existing.ID.String(), rec.Header().Get("Location"))
}

func TestHandle_InvalidBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "not a json", body: `{"user_id":`},
		{name: "unknown field", body: `{"surge": true}`},
		{name: "wrong type", body: `{"price": "free"}`},
		{name: "two objects", body: validBody + validBody},
		{name: "invalid uuid", body: `{"user_id": "not-a-uuid"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := NewMockusecase(gomock.NewController(t))

			rec := perform(t, mock, tt.body, api.CreateTripParams{})

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Equal(t, "invalid_request", decodeProblem(t, rec).Code)
		})
	}
}

func TestHandle_UsecaseErrors(t *testing.T) {
	t.Parallel()

	const dbDetails = "dial tcp 10.0.0.1:5432: connection refused"

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "driver busy", err: trip.ErrDriverBusy, wantStatus: http.StatusConflict, wantCode: "driver_busy"},
		{name: "idempotency conflict", err: trip.ErrIdempotencyConflict, wantStatus: http.StatusConflict, wantCode: "idempotency_conflict"},
		{
			name:       "validation",
			err:        &trip.ValidationError{Fields: []trip.FieldError{{Field: "user_id", Message: "must not be empty"}}},
			wantStatus: http.StatusBadRequest,
			wantCode:   "invalid_request",
		},
		{name: "repository failure", err: errors.New(dbDetails), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := NewMockusecase(gomock.NewController(t))
			mock.EXPECT().Do(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, tt.err)

			rec := perform(t, mock, validBody, api.CreateTripParams{})

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantCode, decodeProblem(t, rec).Code)
			assert.NotContains(t, rec.Body.String(), dbDetails)
		})
	}
}
