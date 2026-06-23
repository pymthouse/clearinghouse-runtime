package main

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"
)

const (
	customerKeyPrefix = "ch_"
	maxSegmentLength  = 120
)

var base32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

func validateSegment(name string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s must be non-empty", name)
	}
	if len(trimmed) > maxSegmentLength {
		return fmt.Errorf("%s must be <= %d chars", name, maxSegmentLength)
	}
	if strings.Contains(trimmed, ":") {
		return fmt.Errorf("%s cannot contain ':'", name)
	}
	return nil
}

func buildAuthID(tenantID string, clientID string, externalUserID string) (string, error) {
	if err := validateSegment("tenant_id", tenantID); err != nil {
		return "", err
	}
	if err := validateSegment("client_id", clientID); err != nil {
		return "", err
	}
	if err := validateSegment("external_user_id", externalUserID); err != nil {
		return "", err
	}
	return strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(clientID) + ":" + strings.TrimSpace(externalUserID), nil
}

func parseAuthID(authID string) (string, string, string, bool) {
	trimmed := strings.TrimSpace(authID)
	if trimmed == "" {
		return "", "", "", false
	}
	first := strings.Index(trimmed, ":")
	if first <= 0 || first >= len(trimmed)-1 {
		return "", "", "", false
	}
	secondOffset := strings.Index(trimmed[first+1:], ":")
	if secondOffset <= 0 {
		return "", "", "", false
	}
	second := first + 1 + secondOffset
	if second >= len(trimmed)-1 {
		return "", "", "", false
	}
	return trimmed[:first], trimmed[first+1 : second], trimmed[second+1:], true
}

func parseLegacyAuthID(authID string) (string, string, bool) {
	trimmed := strings.TrimSpace(authID)
	colon := strings.Index(trimmed, ":")
	if colon <= 0 || colon >= len(trimmed)-1 {
		return "", "", false
	}
	return trimmed[:colon], trimmed[colon+1:], true
}

func buildCustomerKey(tenantID string, clientID string, externalUserID string) (string, error) {
	authID, err := buildAuthID(tenantID, clientID, externalUserID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(authID))
	return customerKeyPrefix + strings.ToLower(base32NoPadding.EncodeToString(sum[:])), nil
}
