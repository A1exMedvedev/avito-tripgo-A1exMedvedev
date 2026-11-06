package middleware

import (
	"log/slog"
	"net/http"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func Logging(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := chimiddleware.NewWrapResponseWriter(w, r.ProtoMajor)
			started := time.Now()
			completed := false

			defer func() {
				status := responseStatus(ww, completed)

				log.LogAttrs(r.Context(), logLevel(status), "request handled",
					slog.String("method", r.Method),
					slog.String("route", routePattern(r)),
					slog.Int("status", status),
					slog.Int64("duration_ms", time.Since(started).Milliseconds()),
				)
			}()

			next.ServeHTTP(ww, r)

			completed = true
		})
	}
}

func logLevel(status int) slog.Level {
	if status >= http.StatusInternalServerError {
		return slog.LevelError
	}

	return slog.LevelInfo
}
