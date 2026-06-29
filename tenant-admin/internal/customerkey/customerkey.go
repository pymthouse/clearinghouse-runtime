// Package customerkey builds the stable OpenMeter customer / event-subject key for an
// (app, external user) pair. The format is deliberately identical to builder-sdk and
// pymthouse — `${clientId}:${externalUserId}` — so that the subject the collector emits
// on usage events attributes to the customer ensured here.
//
// See builder-sdk authIdFromIdentity (`${client_id}:${usage_subject}`) and pymthouse
// buildOpenMeterCustomerKey. Do not change this without changing both of those.
package customerkey

import (
	"fmt"
	"strings"
)

// Build returns `${clientId}:${externalUserId}`.
func Build(clientID, externalUserID string) (string, error) {
	client := strings.TrimSpace(clientID)
	external := strings.TrimSpace(externalUserID)
	if client == "" || external == "" {
		return "", fmt.Errorf("clientId and externalUserId must be non-empty")
	}
	return client + ":" + external, nil
}

// Parse splits a customer key / event subject on its first colon, matching the
// first-colon semantics used by builder-sdk and pymthouse.
func Parse(key string) (clientID, externalUserID string, ok bool) {
	trimmed := strings.TrimSpace(key)
	idx := strings.Index(trimmed, ":")
	if idx <= 0 || idx >= len(trimmed)-1 {
		return "", "", false
	}
	return trimmed[:idx], trimmed[idx+1:], true
}
