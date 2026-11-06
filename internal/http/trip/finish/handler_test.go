package finish_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/finish"
)

func perform(t *testing.T, tr trip.Trip, usecaseErr error) *httptest.ResponseRecorder {
	t.Helper()

	id := uuid.New()
	mock := NewMockusecase(gomock.NewController(t))
	mock.EXPECT().Do(gomock.Any(), id).Return(tr, usecaseErr)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+id.String()+"/finish", nil)

	finish.New(mock, slog.New(slog.DiscardHandler)).Handle(rec, req, id)

	return rec
}

func TestHandle_OK(t *testing.T) {
	t.Parallel()

	finishedAt := time.Date(2026, 8, 24, 12, 18, 0, 0, time.UTC)
	tr := trip.Trip{ID: uuid.New(), Status: trip.StatusCompleted, FinishedAt: &finishedAt}

	rec := perform(t, tr, nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	var got map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "completed", got["status"])
	assert.Equal(t, "2026-08-24T12:18:00Z", got["finished_at"])
}

func TestHandle_Errors(t *testing.T) {
	t.Parallel()

	const dbDetails = "connection refused"

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "already completed", err: trip.ErrTripCompleted, wantStatus: http.StatusConflict, wantCode: "trip_completed"},
		{name: "not found", err: trip.ErrTripNotFound, wantStatus: http.StatusNotFound, wantCode: "trip_not_found"},
		{name: "repository failure", err: errors.New(dbDetails), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := perform(t, trip.Trip{}, tt.err)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.NotContains(t, rec.Body.String(), dbDetails)

			var problem api.Problem
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			assert.Equal(t, tt.wantCode, problem.Code)
		})
	}
}
