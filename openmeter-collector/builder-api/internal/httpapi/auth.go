package httpapi

import (
	"encoding/base64"
	"net/http"
	"strings"
)

// M2MAuth validates HTTP Basic auth against the configured signer M2M client.
func M2MAuth(r *http.Request, expectedClientID, expectedSecret string) bool {
	username, password, ok := r.BasicAuth()
	if !ok {
		auth := strings.TrimSpace(r.Header.Get("Authorization"))
		if !strings.HasPrefix(auth, "Basic ") {
			return false
		}
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(auth, "Basic "))
		if err != nil {
			return false
		}
		parts := strings.SplitN(string(decoded), ":", 2)
		if len(parts) != 2 {
			return false
		}
		username, password = parts[0], parts[1]
	}
	return username == expectedClientID && password == expectedSecret
}

// BearerToken extracts the bearer token from Authorization header.
func BearerToken(r *http.Request) string {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}
