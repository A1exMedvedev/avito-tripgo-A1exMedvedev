package create_test

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
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/trip/create"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

const validBody = `{
	"user_id": "00000000-0000-4000-8000-000000000002",
	"driver_id": "00000000-0000-4000-8000-000000000003",
	"start_point": {"latitude": 59.9398, "longitude": 30.3146},
	"end_point": {"latitude": 59.929, "longitude": 30.3626},
	"price": 1450
}`

const createdJSON = `{
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

const codeInvalidRequest = "invalid_request"

type mocks struct {
	usecase *Mockusecase
	log     *Mocklogger
}

func TestHandler_Handle(t *testing.T) {
	key := uuid.MustParse("00000000-0000-4000-8000-000000000004")
	params := trip.Params{
		UserID:   uuid.MustParse("00000000-0000-4000-8000-000000000002"),
		DriverID: uuid.MustParse("00000000-0000-4000-8000-000000000003"),
		Start:    trip.Point{Latitude: 59.9398, Longitude: 30.3146},
		End:      trip.Point{Latitude: 59.929, Longitude: 30.3626},
		Price:    1450,
	}
	created := trip.Trip{
		ID:        uuid.MustParse("00000000-0000-4000-8000-000000000001"),
		UserID:    params.UserID,
		DriverID:  params.DriverID,
		Start:     params.Start,
		End:       params.End,
		Price:     params.Price,
		Status:    trip.StatusActive,
		StartedAt: time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC),
	}
	location := "/api/v1/trips/" + created.ID.String()

	tests := []struct {
		name         string
		setup        func(m mocks)
		body         string
		params       api.CreateTripParams
		wantStatus   int
		wantBody     string
		wantCode     string
		wantLocation string
	}{
		{
			name: "успех, поездка создана",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), model.CreateTripInput{Params: params}).
					Return(model.CreateTripResult{Trip: created}, nil)
			},
			body:         validBody,
			wantStatus:   http.StatusCreated,
			wantBody:     createdJSON,
			wantLocation: location,
		},
		{
			name: "повтор с тем же ключом идемпотентности → 200",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), model.CreateTripInput{Params: params, IdempotencyKey: &key}).
					Return(model.CreateTripResult{Trip: created, Replayed: true}, nil)
			},
			body:         validBody,
			params:       api.CreateTripParams{IdempotencyKey: &key},
			wantStatus:   http.StatusOK,
			wantBody:     createdJSON,
			wantLocation: location,
		},
		{
			name:       "невалидный JSON → 400",
			setup:      func(mocks) {},
			body:       `{"user_id":`,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "неизвестное поле → 400",
			setup:      func(mocks) {},
			body:       `{"surge": true}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "поле неверного типа → 400",
			setup:      func(mocks) {},
			body:       `{"price": "free"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "невалидный uuid → 400",
			setup:      func(mocks) {},
			body:       `{"user_id": "not-a-uuid"}`,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name:       "два объекта в теле → 400",
			setup:      func(mocks) {},
			body:       validBody + validBody,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "ошибка валидации в домене → 400",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), gomock.Any()).
					Return(model.CreateTripResult{}, &trip.ValidationError{Fields: []trip.FieldError{{Field: "user_id", Message: "must not be empty"}}})
			},
			body:       validBody,
			wantStatus: http.StatusBadRequest,
			wantCode:   codeInvalidRequest,
		},
		{
			name: "водитель занят → 409",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, trip.ErrDriverBusy)
			},
			body:       validBody,
			wantStatus: http.StatusConflict,
			wantCode:   "driver_busy",
		},
		{
			name: "ключ идемпотентности с другим телом → 409",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, trip.ErrIdempotencyConflict)
			},
			body:       validBody,
			wantStatus: http.StatusConflict,
			wantCode:   "idempotency_conflict",
		},
		{
			name: "ошибка репозитория → 500 без деталей, ошибка в логе",
			setup: func(m mocks) {
				m.usecase.EXPECT().Do(gomock.Any(), gomock.Any()).Return(model.CreateTripResult{}, assert.AnError)
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
			req := httptest.NewRequest(http.MethodPost, "/api/v1/trips", strings.NewReader(tt.body))

			create.New(m.usecase, m.log).Handle(rec, req, tt.params)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantLocation, rec.Header().Get("Location"))
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
