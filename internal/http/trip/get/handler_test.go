package get_test

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/get"
)

func perform(t *testing.T, tr trip.Trip, usecaseErr error) *httptest.ResponseRecorder {
	t.Helper()

	id := uuid.New()
	mock := NewMockusecase(gomock.NewController(t))
	mock.EXPECT().Do(gomock.Any(), id).Return(tr, usecaseErr)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+id.String(), nil)

	get.New(mock, slog.New(slog.DiscardHandler)).Handle(rec, req, id)

	return rec
}

func TestHandle_OK(t *testing.T) {
	t.Parallel()

	tr := trip.Trip{ID: uuid.New(), Status: trip.StatusActive, Price: 1450}

	rec := perform(t, tr, nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	var got api.Trip
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, tr.ID, got.Id)
	assert.Equal(t, tr.Price, got.Price)
}

func TestHandle_NotFound(t *testing.T) {
	t.Parallel()

	rec := perform(t, trip.Trip{}, trip.ErrTripNotFound)

	assert.Equal(t, http.StatusNotFound, rec.Code)

	var problem api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "trip_not_found", problem.Code)
}

func TestHandle_RepositoryFailure_HidesDetails(t *testing.T) {
	t.Parallel()

	const dbDetails = "connection refused"

	rec := perform(t, trip.Trip{}, errors.New(dbDetails))

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), dbDetails)

	var problem api.Problem
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, "internal_error", problem.Code)
}
