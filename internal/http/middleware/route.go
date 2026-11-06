package middleware

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func routePattern(r *http.Request) string {
	rctx := chi.RouteContext(r.Context())
	if rctx == nil {
		return ""
	}

	return rctx.RoutePattern()
}

func responseStatus(ww chimiddleware.WrapResponseWriter, completed bool) int {
	if !completed {
		return http.StatusInternalServerError
	}

	if status := ww.Status(); status != 0 {
		return status
	}

	return http.StatusOK
}
