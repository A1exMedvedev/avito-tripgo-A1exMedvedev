package response

import (
	"encoding/json"
	"net/http"

	"github.com/A1exMedvedev/avito-tripgo-A1exMedvedev/api"
)

func JSON(w http.ResponseWriter, status int, body any) {
	write(w, "application/json", status, body)
}

func Problem(w http.ResponseWriter, r *http.Request, body api.Problem) {
	body.Instance = new(r.URL.Path)
	write(w, "application/problem+json", int(body.Status), body)
}

func write(w http.ResponseWriter, contentType string, status int, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
