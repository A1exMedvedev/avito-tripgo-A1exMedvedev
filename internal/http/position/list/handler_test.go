package list_test

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
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/position/list"
)

const routeJSON = `{"positions": [
	{"id": 1, "trip_id": "00000000-0000-4000-8000-000000000001", "coordinates": {"latitude": 59.93, "longitude": 30.33}, "recorded_at": "2026-08-24T12:00:00Z"},
	{"id": 2, "trip_id": "00000000-0000-4000-8000-000000000001", "coordinates": {"latitude": 59.94, "longitude": 30.34}, "recorded_at": "2026-08-24T12:00:05Z"}
]}`

type mocks struct {
	usecase *Mockusecase
	log     *Mocklogger
}

func TestHandler_Handle(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	startedAt := time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC)
	route := []trip.Position{
		{ID: 1, TripID: tripID, Point: trip.Point{Latitude: 59.93, Longitude: 30.33}, RecordedAt: startedAt},
		{ID: 2, TripID: tripID, Point: trip.Point{Latitude: 59.94, Longitude: 30.34}, RecordedAt: startedAt.Add(5 * time.Second)},
	}

	tests := []struct {
		name       string
		setup      func(m mocks)
		wantStatus int
		wantBody   string
		wantCode   string
	}{
		{
			name: "успех, маршрут",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), tripID).Return(route, nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   routeJSON,
		},
		{
			name: "успех, точек нет — пустой массив, а не null",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), tripID).Return([]trip.Position{}, nil)
			},
			wantStatus: http.StatusOK,
			wantBody:   `{"positions": []}`,
		},
		{
			name: "поездка не найдена → 404",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), tripID).Return(nil, trip.ErrTripNotFound)
			},
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name: "ошибка репозитория → 500 без деталей, ошибка в логе",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), tripID).Return(nil, assert.AnError)
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
			req := httptest.NewRequest(http.MethodGet, "/api/v1/trips/"+tripID.String()+"/positions", nil)

			list.New(m.usecase, m.log).Handle(rec, req, tripID)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.NotContains(t, rec.Body.String(), assert.AnError.Error())

			if tt.wantCode == "" {
				assert.JSONEq(t, tt.wantBody, rec.Body.String())
				return
			}

			var problem api.Problem
			if assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem)) {
				assert.Equal(t, tt.wantCode, problem.Code)
			}
		})
	}
}
