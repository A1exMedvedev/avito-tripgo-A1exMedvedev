package get

import (
	"log/slog"
	"net/http"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/domainerror"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/response"
)

type Handler struct {
	usecase usecase
	log     *slog.Logger
}

func New(u usecase, log *slog.Logger) *Handler {
	return &Handler{usecase: u, log: log}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.usecase.Do(r.Context(), tripID)
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	response.JSON(w, http.StatusOK, response.Trip(t))
}
