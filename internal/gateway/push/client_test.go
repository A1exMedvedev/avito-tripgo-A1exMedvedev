package push_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/propagation"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/goleak"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/gateway/push"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestClient_RequestPosition(t *testing.T) {
	driverID := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	tripID := uuid.MustParse("00000000-0000-4000-8000-000000000002")

	tests := []struct {
		name    string
		handler http.HandlerFunc
		block   bool
		closed  bool
		wantErr assert.ErrorAssertionFunc
	}{
		{
			name: "успех, команда принята",
			handler: func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&body)) {
					return
				}

				assert.Equal(t, "/api/v1/push", r.URL.Path)
				assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
				assert.Equal(t, "REQUEST_POSITION", body["kind"])
				assert.Equal(t, driverID.String(), body["recipient_id"])
				assert.Equal(t, map[string]any{"trip_id": tripID.String()}, body["data"])
				assert.NotEmpty(t, body["request_id"])

				w.WriteHeader(http.StatusAccepted)
				_, _ = w.Write([]byte(`{"message_id":"1","accepted_at":"2026-08-24T12:00:00Z"}`))
			},
			wantErr: assert.NoError,
		},
		{
			name:    "400 — запрос отклонён",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusBadRequest) },
			wantErr: errorIs(push.ErrRejected),
		},
		{
			name:    "429 — запрос отклонён",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTooManyRequests) },
			wantErr: errorIs(push.ErrRejected),
		},
		{
			name:    "503 — сервис недоступен",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			wantErr: errorIs(push.ErrUnavailable),
		},
		{
			name:  "таймаут ответа",
			block: true,
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...any) bool {
				return assert.Error(t, err, msgAndArgs...) &&
					assert.NotErrorIs(t, err, push.ErrRejected, msgAndArgs...) &&
					assert.NotErrorIs(t, err, push.ErrUnavailable, msgAndArgs...)
			},
		},
		{
			name:    "ошибка транспорта отличается от ответов с кодом",
			handler: func(http.ResponseWriter, *http.Request) {},
			closed:  true,
			wantErr: func(t assert.TestingT, err error, msgAndArgs ...any) bool {
				return assert.Error(t, err, msgAndArgs...) &&
					assert.NotErrorIs(t, err, push.ErrRejected, msgAndArgs...) &&
					assert.NotErrorIs(t, err, push.ErrUnavailable, msgAndArgs...)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := tt.handler
			release := make(chan struct{})

			if tt.block {
				handler = func(http.ResponseWriter, *http.Request) { <-release }
			}

			server := httptest.NewServer(handler)
			if tt.closed {
				server.Close()
			} else {
				t.Cleanup(server.Close)
			}

			client := push.New(server.URL, 200*time.Millisecond, tracenoop.NewTracerProvider(), propagation.TraceContext{})
			t.Cleanup(client.Close)

			err := client.RequestPosition(context.Background(), driverID, tripID)
			close(release)

			tt.wantErr(t, err)
		})
	}
}

func TestClient_RespectsContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)

	client := push.New(server.URL, time.Second, tracenoop.NewTracerProvider(), propagation.TraceContext{})
	t.Cleanup(client.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := client.RequestPosition(ctx, uuid.New(), uuid.New())

	require.ErrorIs(t, err, context.Canceled)
}

func errorIs(target error) assert.ErrorAssertionFunc {
	return func(t assert.TestingT, err error, msgAndArgs ...any) bool {
		return assert.ErrorIs(t, err, target, msgAndArgs...)
	}
}
