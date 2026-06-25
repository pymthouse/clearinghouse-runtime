package billingidentity

import (
	"crypto/sha256"
	"encoding/base32"
	"fmt"
	"strings"
)

const (
	CustomerKeyPrefix = "ch_"
	MaxSegmentLength  = 120
)

var base32NoPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

func ValidateSegment(name string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s must be non-empty", name)
	}
	if len(trimmed) > MaxSegmentLength {
		return fmt.Errorf("%s must be <= %d chars", name, MaxSegmentLength)
	}
	if strings.Contains(trimmed, ":") {
		return fmt.Errorf("%s cannot contain ':'", name)
	}
	return nil
}

func BuildAuthID(tenantID string, clientID string, externalUserID string) (string, error) {
	if err := ValidateSegment("tenantId", tenantID); err != nil {
		return "", err
	}
	if err := ValidateSegment("clientId", clientID); err != nil {
		return "", err
	}
	if err := ValidateSegment("externalUserId", externalUserID); err != nil {
		return "", err
	}
	return strings.TrimSpace(tenantID) + ":" + strings.TrimSpace(clientID) + ":" + strings.TrimSpace(externalUserID), nil
}

func ParseAuthID(authID string) (string, string, string, bool) {
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

func ParseLegacyAuthID(authID string) (string, string, bool) {
	trimmed := strings.TrimSpace(authID)
	if trimmed == "" {
		return "", "", false
	}
	colon := strings.Index(trimmed, ":")
	if colon <= 0 || colon >= len(trimmed)-1 {
		return "", "", false
	}
	return trimmed[:colon], trimmed[colon+1:], true
}

func BuildCustomerKey(tenantID string, clientID string, externalUserID string) (string, error) {
	authID, err := BuildAuthID(tenantID, clientID, externalUserID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(authID))
	return CustomerKeyPrefix + strings.ToLower(base32NoPadding.EncodeToString(sum[:])), nil
}
