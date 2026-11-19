package router

import (
	"net/http"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
)

type Handlers struct {
	Create createHandler
	Get    tripHandler
	Finish tripHandler
	Health healthHandler
}

var _ api.ServerInterface = (*server)(nil)

type server struct {
	handlers Handlers
}

func (s *server) CreateTrip(w http.ResponseWriter, r *http.Request, params api.CreateTripParams) {
	s.handlers.Create.Handle(w, r, params)
}

func (s *server) GetTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.handlers.Get.Handle(w, r, tripID)
}

func (s *server) FinishTrip(w http.ResponseWriter, r *http.Request, tripID api.TripId) {
	s.handlers.Finish.Handle(w, r, tripID)
}

func (s *server) Health(w http.ResponseWriter, r *http.Request) {
	s.handlers.Health.Live(w, r)
}

func (s *server) Ready(w http.ResponseWriter, r *http.Request) {
	s.handlers.Health.Ready(w, r)
}
