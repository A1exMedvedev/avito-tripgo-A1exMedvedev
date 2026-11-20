package save_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/position/save"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

const (
	codeInvalidRequest = "invalid_request"

	validBody = `{"coordinates": {"latitude": 59.935, "longitude": 30.335}, "recorded_at": "2026-08-24T12:00:05Z"}`

	savedJSON = `{
		"id": 42,
		"trip_id": "00000000-0000-4000-8000-000000000001",
		"coordinates": {"latitude": 59.935, "longitude": 30.335},
		"recorded_at": "2026-08-24T12:00:05Z"
	}`
)

type mocks struct {
	usecase *Mockusecase
	log     *Mocklogger
}

func TestHandler_Handle(t *testing.T) {
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	recordedAt := time.Date(2026, 8, 24, 12, 0, 5, 0, time.UTC)
	point := trip.Point{Latitude: 59.935, Longitude: 30.335}
	in := model.SavePositionInput{TripID: tripID, Point: point, RecordedAt: recordedAt}

	tests := []struct {
		name       string
		setup      func(m mocks)
		body       string
		wantStatus int
		wantBody   string
		wantCode   string
	}{
		{
			name: "успех, координата сохранена",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), in).
					Return(trip.Position{ID: 42, TripID: tripID, Point: point, RecordedAt: recordedAt}, nil)
			},
			body:       validBody,
			wantStatus: http.StatusCreated,
			wantBody:   savedJSON,
		},
		{
			name:       "невалидный JSON → 400",
			setup:      func(mocks) {},
			body:       `{"coordinates":`,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "recorded_at не разобрался → 400",
			setup:      func(mocks) {},
			body:       `{"coordinates": {"latitude": 1, "longitude": 1}, "recorded_at": "вчера"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "неизвестное поле → 400",
			setup:      func(mocks) {},
			body:       `{"speed": 10}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "координаты вне диапазона или время в будущем → 400",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), in).
					Return(trip.Position{}, &trip.ValidationError{Fields: []trip.FieldError{{Field: "recorded_at", Message: "in the future"}}})
			},
			body:       validBody,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "поездка не найдена → 404",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), in).Return(trip.Position{}, trip.ErrTripNotFound)
			},
			body:       validBody,
			wantStatus: http.StatusNotFound,
			wantCode:   "trip_not_found",
		},
		{
			name: "поездка завершена → 409",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), in).Return(trip.Position{}, trip.ErrTripCompleted)
			},
			body:       validBody,
			wantStatus: http.StatusConflict,
			wantCode:   "trip_completed",
		},
		{
			name: "ошибка репозитория → 500 без деталей, ошибка в логе",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), in).Return(trip.Position{}, assert.AnError)
				m.log.EXPECT().ErrorContext(gomock.Any(), "request failed", gomock.Any())
			},
			body:       validBody,
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips/"+tripID.String()+"/positions", strings.NewReader(tt.body))

			save.New(m.usecase, m.log).Handle(rec, req, tripID)

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
