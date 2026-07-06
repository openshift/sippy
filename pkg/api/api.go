package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/openshift/sippy/pkg/filter"
)

func RespondWithJSON(statusCode int, w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(statusCode)

	if jsonString, ok := data.(string); ok {
		fmt.Fprint(w, jsonString) //nolint:gosec // G705: jsonString is pre-marshaled JSON from internal sources, response Content-Type is application/json
		return
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		fmt.Fprintf(w, `{"message": "could not marshal results: %s"}`, err)
	}
}

// RespondWithError writes an error JSON response, returning HTTP 400 for
// filter validation errors and 500 for everything else.
func RespondWithError(w http.ResponseWriter, msg string, err error) {
	code := http.StatusInternalServerError
	var validationErr *filter.FilterValidationError
	if errors.As(err, &validationErr) {
		code = http.StatusBadRequest
	}
	RespondWithJSON(code, w, map[string]any{"code": code, "message": msg + err.Error()})
}
