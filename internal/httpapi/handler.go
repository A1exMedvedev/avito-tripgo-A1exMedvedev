package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/oapi-codegen/nullable"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/internal/trip"
)

type TripService interface {
	Create(ctx context.Context, p trip.CreateParams, idempotencyKey *uuid.UUID) (t trip.Trip, replayed bool, err error)
	Get(ctx context.Context, id uuid.UUID) (trip.Trip, error)
	Finish(ctx context.Context, id uuid.UUID) (trip.Trip, error)
}

type Pinger interface {
	Ping(ctx context.Context) error
}

type Handler struct {
	trips        TripService
	db           Pinger
	readyTimeout time.Duration
	log          *slog.Logger
}

var _ api.ServerInterface = (*Handler)(nil)

func NewHandler(trips TripService, db Pinger, readyTimeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{trips: trips, db: db, readyTimeout: readyTimeout, log: log}
}

func (h *Handler) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	var body api.TripData

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(&body); err != nil {
		writeProblem(w, r, problemInvalidRequest, "invalid JSON body: "+err.Error())
		return
	}

	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, r, problemInvalidRequest, "request body must contain a single JSON object")
		return
	}

	t, replayed, err := h.trips.Create(r.Context(), trip.CreateParams{
		UserID:   body.UserId,
		DriverID: body.DriverId,
		Start:    trip.Point{Latitude: body.StartPoint.Latitude, Longitude: body.StartPoint.Longitude},
		End:      trip.Point{Latitude: body.EndPoint.Latitude, Longitude: body.EndPoint.Longitude},
		Price:    body.Price,
	}, params.IdempotencyKey)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	w.Header().Set("Location", "/api/v1/trips/"+t.ID.String())

	if replayed {
		writeJSON(w, http.StatusOK, toAPITrip(t))
		return
	}

	writeJSON(w, http.StatusCreated, toAPITrip(t))
}

func (h *Handler) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.trips.Get(r.Context(), tripID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(t))
}

func (h *Handler) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	t, err := h.trips.Finish(r.Context(), tripID)
	if err != nil {
		h.writeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPITrip(t))
}

func (h *Handler) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) Ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), h.readyTimeout)
	defer cancel()

	if err := h.db.Ping(ctx); err != nil {
		h.log.WarnContext(ctx, "readiness check failed", slog.Any("error", err))
		writeJSON(w, http.StatusServiceUnavailable, api.HealthResponse{Status: api.Unavailable})

		return
	}

	writeJSON(w, http.StatusOK, api.HealthResponse{Status: api.Ok})
}

func (h *Handler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, trip.ErrInvalidTrip):
		writeProblem(w, r, problemInvalidRequest, err.Error())
	case errors.Is(err, trip.ErrTripNotFound):
		writeProblem(w, r, problemTripNotFound, "")
	case errors.Is(err, trip.ErrTripCompleted):
		writeProblem(w, r, problemTripCompleted, "")
	case errors.Is(err, trip.ErrDriverBusy):
		writeProblem(w, r, problemDriverBusy, "")
	case errors.Is(err, trip.ErrIdempotencyConflict):
		writeProblem(w, r, problemIdempotencyConflict, "")
	default:
		h.log.ErrorContext(r.Context(), "request failed",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Any("error", err),
		)
		writeProblem(w, r, problemInternal, "")
	}
}

func toAPITrip(t trip.Trip) api.Trip {
	finishedAt := nullable.NewNullNullable[time.Time]()
	if t.FinishedAt != nil {
		finishedAt = nullable.NewNullableWithValue(*t.FinishedAt)
	}

	return api.Trip{
		Id:             t.ID,
		UserId:         t.UserID,
		DriverId:       t.DriverID,
		StartPoint:     api.Coordinates{Latitude: t.Start.Latitude, Longitude: t.Start.Longitude},
		EndPoint:       api.Coordinates{Latitude: t.End.Latitude, Longitude: t.End.Longitude},
		Price:          t.Price,
		Status:         api.TripStatus(t.Status),
		StartedAt:      t.StartedAt,
		FinishedAt:     finishedAt,
		LastPositionAt: nullable.NewNullNullable[time.Time](),
	}
}
