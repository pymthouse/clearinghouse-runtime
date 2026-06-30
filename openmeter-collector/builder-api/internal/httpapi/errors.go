package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
)

// OAuthError is an OAuth 2.0-style error response body.
type OAuthError struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
	CorrelationID    string `json:"correlation_id,omitempty"`
}

// APIError is a simple JSON error for non-OAuth routes.
type APIError struct {
	Error string `json:"error"`
}

func newCorrelationID() string {
	return uuid.NewString()
}

func writeOAuthError(w http.ResponseWriter, status int, code, description, correlationID string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(OAuthError{
		Error:            code,
		ErrorDescription: description,
		CorrelationID:    correlationID,
	})
}

func writeAPIError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIError{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
