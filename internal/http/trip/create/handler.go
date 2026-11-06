package create

import (
	"encoding/json"
	"errors"
	"io"
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

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var body api.TripData

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&body); err != nil {
		response.Problem(w, r, domainerror.InvalidRequest("invalid JSON body: "+err.Error()))
		return
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		response.Problem(w, r, domainerror.InvalidRequest("request body must contain a single JSON object"))
		return
	}

	result, err := h.usecase.Do(r.Context(), toInput(body, params))
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+result.Trip.ID.String())

	status := http.StatusCreated
	if result.Replayed {
		status = http.StatusOK
	}

	response.JSON(w, status, response.Trip(result.Trip))
}
