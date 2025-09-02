package v1

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// MARK: respondWithSuccess
// Handle API responses that are successful.
func (a *APIServer) respondWithSuccess(w http.ResponseWriter, message string, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	response := APIResponse{
		Success: true,
		Message: message,
		Data:    data,
	}
	json.NewEncoder(w).Encode(response)
}

// MARK: respondWithError
// Handle API responses that are errors.
func (a *APIServer) respondWithError(w http.ResponseWriter, statusCode int, errorMessage string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	response := APIResponse{
		Success: false,
		Error:   errorMessage,
	}
	json.NewEncoder(w).Encode(response)
}

// MARK: parsePaginationParams
// Parse pagination parameters.
func (a *APIServer) parsePaginationParams(r *http.Request) (int, int) {
	limit := 100
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 1000 {
			limit = parsed
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	return limit, offset
}
