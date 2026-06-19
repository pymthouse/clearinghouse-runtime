package provision

import "strings"

// EnsureRequest accepts flexible identity field aliases from webhook and collector callers.
type EnsureRequest struct {
	ClientID       string         `json:"client_id"`
	ClientIDAlt    string         `json:"clientId"`
	ExternalUserID string         `json:"external_user_id"`
	ExternalUserAlt  string         `json:"externalUserId"`
	AuthID         string         `json:"auth_id"`
	AuthIDAlt        string         `json:"authId"`
	Subject        string         `json:"subject"`
	Data           map[string]any `json:"data"`
}

func stringField(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := body[key]; ok {
			if value, ok := raw.(string); ok {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

// ParseIdentity resolves client_id and external_user_id from request body aliases.
func ParseIdentity(body EnsureRequest) (clientID, externalUserID string, ok bool) {
	data := body.Data
	if data == nil {
		data = map[string]any{}
	}

	clientID = FirstNonEmpty(
		body.ClientID,
		body.ClientIDAlt,
		stringField(data, "client_id", "clientId"),
	)
	externalUserID = FirstNonEmpty(
		body.ExternalUserID,
		body.ExternalUserAlt,
		stringField(data, "external_user_id", "externalUserId"),
	)
	if clientID != "" && externalUserID != "" {
		return clientID, externalUserID, true
	}

	authID := FirstNonEmpty(
		body.AuthID,
		body.AuthIDAlt,
		stringField(data, "auth_id", "authId"),
		body.Subject,
	)
	colon := strings.Index(authID, ":")
	if colon > 0 && colon < len(authID)-1 {
		return authID[:colon], authID[colon+1:], true
	}
	return "", "", false
}

// FirstNonEmpty returns the first non-empty trimmed string.
func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
