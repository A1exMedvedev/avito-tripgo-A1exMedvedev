package router

import (
	"net/http"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
)

type (
	createHandler interface {
		Handle(w http.ResponseWriter, r *http.Request, params api.CreateTripParams)
	}

	tripHandler interface {
		Handle(w http.ResponseWriter, r *http.Request, tripID api.TripId)
	}

	healthHandler interface {
		Live(w http.ResponseWriter, r *http.Request)
		Ready(w http.ResponseWriter, r *http.Request)
	}
)
