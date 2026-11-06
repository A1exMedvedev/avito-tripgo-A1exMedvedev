package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/domainerror"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/response"
)

var errPanic = errors.New("panic recovered")

func Recover(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				p := recover()
				if p == nil {
					return
				}

				if p == http.ErrAbortHandler {
					panic(p)
				}

				log.ErrorContext(r.Context(), errPanic.Error(),
					slog.Any("panic", p),
					slog.String("stack", string(debug.Stack())),
				)

				_, body := domainerror.Problem(errPanic)
				response.Problem(w, r, body)
			}()

			next.ServeHTTP(w, r)
		})
	}
}
