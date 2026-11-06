package health_test

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/health"
)

func TestLive_DoesNotTouchDatabase(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	h := health.New(NewMockpinger(gomock.NewController(t)), time.Second, slog.New(slog.DiscardHandler))

	h.Live(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"status":"ok"}`, rec.Body.String())
}

func TestReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
		wantBody   string
	}{
		{name: "database is up", wantStatus: http.StatusOK, wantBody: `{"status":"ok"}`},
		{name: "database is down", pingErr: errors.New("refused"), wantStatus: http.StatusServiceUnavailable, wantBody: `{"status":"unavailable"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			db := NewMockpinger(gomock.NewController(t))
			db.EXPECT().Ping(gomock.Any()).Return(tt.pingErr)

			rec := httptest.NewRecorder()
			h := health.New(db, time.Second, slog.New(slog.DiscardHandler))

			h.Ready(rec, httptest.NewRequest(http.MethodGet, "/ready", nil))

			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.JSONEq(t, tt.wantBody, rec.Body.String())
		})
	}
}
