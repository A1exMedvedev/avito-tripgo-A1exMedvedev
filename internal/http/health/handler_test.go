package health_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/health"
)

type mocks struct {
	db  *Mockpinger
	log *Mocklogger
}

func TestHandler(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(m mocks)
		handle     func(h *health.Handler) http.HandlerFunc
		wantStatus int
		wantBody   string
	}{
		{
			name:       "liveness не ходит в бд",
			setup:      func(mocks) {},
			handle:     func(h *health.Handler) http.HandlerFunc { return h.Live },
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name: "readiness, бд доступна",
			setup: func(m mocks) {
				m.db.EXPECT().Ping(gomock.Any()).Return(nil)
			},
			handle:     func(h *health.Handler) http.HandlerFunc { return h.Ready },
			wantStatus: http.StatusOK,
			wantBody:   `{"status":"ok"}`,
		},
		{
			name: "readiness, бд недоступна → 503 и предупреждение в логе",
			setup: func(m mocks) {
				m.db.EXPECT().Ping(gomock.Any()).Return(assert.AnError)
				m.log.EXPECT().WarnContext(gomock.Any(), "readiness check failed", gomock.Any())
			},
			handle:     func(h *health.Handler) http.HandlerFunc { return h.Ready },
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `{"status":"unavailable"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			m := mocks{db: NewMockpinger(ctrl), log: NewMocklogger(ctrl)}
			tt.setup(m)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/", nil)

			tt.handle(health.New(m.db, time.Second, m.log))(rec, req)

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
		})
	}
}
