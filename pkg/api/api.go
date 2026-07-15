package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgconn"
	log "github.com/sirupsen/logrus"
	"google.golang.org/api/googleapi"
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
// database column errors (invalid filter/sort fields) and 500 for everything else.
func RespondWithError(w http.ResponseWriter, msg string, err error) {
	code := http.StatusInternalServerError
	if IsBadRequestError(err) {
		code = http.StatusBadRequest
	}
	log.WithError(err).Error(msg)
	RespondWithJSON(code, w, map[string]any{"code": code, "message": msg})
}

const pgUndefinedColumn = "42703"

// bqInvalidQuery matches BigQuery's error reason for query-level errors
// such as unrecognized column names, invalid syntax, and type mismatches.
const bqInvalidQuery = "invalidQuery"

// ValidationError represents a request validation failure (missing or invalid
// parameters). IsBadRequestError recognizes it so handlers can rely on the
// shared bad-request classification instead of hard-coding HTTP 400.
type ValidationError struct {
	Message string
}

func (e *ValidationError) Error() string {
	return e.Message
}

func IsBadRequestError(err error) bool {
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return true
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUndefinedColumn {
		return true
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusBadRequest &&
		len(apiErr.Errors) > 0 && apiErr.Errors[0].Reason == bqInvalidQuery {
		return true
	}
	return false
}
