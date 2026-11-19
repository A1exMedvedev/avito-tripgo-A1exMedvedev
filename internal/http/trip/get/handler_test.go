package get_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/get"
)

const tripJSON = `{
	"id": "00000000-0000-4000-8000-000000000001",
	"user_id": "00000000-0000-4000-8000-000000000002",
	"driver_id": "00000000-0000-4000-8000-000000000003",
	"start_point": {"latitude": 59.9398, "longitude": 30.3146},
	"end_point": {"latitude": 59.929, "longitude": 30.3626},
	"price": 1450,
	"status": "active",
	"started_at": "2026-08-24T12:00:00Z",
	"finished_at": null,
	"last_position_at": null
}`

type mocks struct {
	usecase *Mockusecase
	log     *Mocklogger
}

func TestHandler_Handle(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	startedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	found := trip.Trip{
		ID:        tripID,
		UserID:    uuid.MustParse("00000000-0000-4000-8000-000000000002"),
		DriverID:  uuid.MustParse("00000000-0000-4000-8000-000000000003"),
		Start:     trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:       trip.Point{Latitude: 59.929, Longitude: 30.3626},
		Price:     1450,
		Status:    trip.StatusActive,
		StartedAt: startedAt,
	}

	tests := []struct {
		name       string
		setup      func(m mocks)
		wantStatus int
		wantBody   string
		wantCode   string
	}{
		{
			name: "успех",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), tripID).Return(found, nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   tripJSON,
		},
		{
			name: "поездка не найдена → 404",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), tripID).Return(trip.Trip{}, trip.ErrTripNotFound)
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name: "ошибка репозитория → 500 без деталей, ошибка в логе",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), tripID).Return(trip.Trip{}, assert.AnError)
				m.log.EXPECT().ErrorContext(gomock.Any(), "request failed", gomock.Any())
			},
			wantStatus: http.StatusInternalServerError,
			wantCode:   "internal_error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := mocks{usecase: NewMockusecase(ctrl), log: NewMocklogger(ctrl)}
			tt.setup(m)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String(), nil)

			get.New(m.usecase, m.log).Handle(rec, req, tripID)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.NotContains(t, rec.Body.String(), assert.AnError.Error())
			assertBody(t, rec, tt.wantBody, tt.wantCode)
		})
	}
}

func assertBody(t *testing.T, rec *httptest.ResponseRecorder, wantBody, wantCode string) {
	t.Helper()

	if wantCode == "" {
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
		assert.JSONEq(t, wantBody, rec.Body.String())

		return
	}

	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

	var problem api.Problem
	if assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem)) {
		assert.Equal(t, wantCode, problem.Code)
	}
}
