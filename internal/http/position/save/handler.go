package save

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/domain/trip"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/domainerror"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/http/response"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/model"
)

type Handler struct {
	usecase usecase
	log     logger
}

func New(u usecase, log logger) *Handler {
	return &Handler{usecase: u, log: log}
}

func (h *Handler) Handle(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	var body api.PositionData

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

	p, err := h.usecase.Do(r.Context(), model.SavePositionInput{
		TripID:     tripID,
		Point:      trip.Point{Latitude: body.Coordinates.Latitude, Longitude: body.Coordinates.Longitude},
		RecordedAt: body.RecordedAt,
	})
	if err != nil {
		domainerror.Write(w, r, h.log, err)
		return
	}

	response.JSON(w, http.StatusCreated, response.Position(p))
}
