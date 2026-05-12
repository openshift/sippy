package api

import (
	"encoding/json"
	"fmt"
	"net/http"
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
