package health

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/response"
)

type Handler struct {
	db           pinger
	readyTimeout time.Duration
	log          *slog.Logger
}

func New(db pinger, readyTimeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{db: db, readyTimeout: readyTimeout, log: log}
}

func (h *Handler) Live(w http.ResponseWriter, _ *http.Request) {
	response.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readyTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.log.WarnContext(ctx, "readiness check failed", slog.Any("error", err))
		response.JSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})

		return
	}

	response.JSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}
