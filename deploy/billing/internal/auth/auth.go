package auth

import (
	"crypto/subtle"
	"net/http"
	"strings"
)

// AuthenticateBearer returns true when the request carries the expected bearer secret.
func AuthenticateBearer(request *http.Request, secret string) bool {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return false
	}

	auth := strings.TrimSpace(request.Header.Get("Authorization"))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "bearer ") {
		token := strings.TrimSpace(auth[7:])
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1 {
			return true
		}
	}

	apiKey := strings.TrimSpace(request.Header.Get("X-Api-Key"))
	if apiKey != "" && subtle.ConstantTimeCompare([]byte(apiKey), []byte(secret)) == 1 {
		return true
	}

	return false
}
