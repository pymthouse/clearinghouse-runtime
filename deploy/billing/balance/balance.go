package balance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/livepeer/clearinghouse/deploy/billing/provision"
)

const entitlementAccessTimeout = 20 * time.Second

var konnectVersionSuffix = regexp.MustCompile(`/v\d+$`)

// Result is the boolean balance gate response for identity-webhook.
type Result struct {
	CustomerKey string `json:"customerKey"`
	FeatureKey  string `json:"featureKey"`
	HasAccess   bool   `json:"hasAccess"`
}

// Checker resolves Konnect entitlement-access for a customer.
type Checker struct {
	provisioner *provision.Provisioner
	baseURL     string
	apiKey      string
	featureKey  string
	client      *http.Client
}

// NewChecker constructs a balance checker.
func NewChecker(
	provisioner *provision.Provisioner,
	baseURL,
	apiKey,
	featureKey string,
) *Checker {
	return &Checker{
		provisioner: provisioner,
		baseURL:     strings.TrimSpace(baseURL),
		apiKey:      strings.TrimSpace(apiKey),
		featureKey:  strings.TrimSpace(featureKey),
		client:      &http.Client{Timeout: entitlementAccessTimeout},
	}
}

// Check returns entitlement access for the given tenant user.
func (c *Checker) Check(ctx context.Context, clientID, externalUserID string) (*Result, error) {
	clientID = strings.TrimSpace(clientID)
	externalUserID = strings.TrimSpace(externalUserID)
	if clientID == "" || externalUserID == "" {
		return nil, fmt.Errorf("clientId and externalUserId must be non-empty")
	}
	if c.featureKey == "" {
		return nil, fmt.Errorf("feature key must be non-empty")
	}

	customerKey := provision.BuildCustomerKey(clientID, externalUserID)
	customerID, err := c.provisioner.FindCustomerByKey(ctx, customerKey)
	if err != nil {
		return nil, fmt.Errorf("finding customer %s: %w", customerKey, err)
	}
	if customerID == "" {
		return &Result{
			CustomerKey: customerKey,
			FeatureKey:  c.featureKey,
			HasAccess:   false,
		}, nil
	}

	hasAccess, err := c.entitlementHasAccess(ctx, customerID, c.featureKey)
	if err != nil {
		return nil, err
	}

	return &Result{
		CustomerKey: customerKey,
		FeatureKey:  c.featureKey,
		HasAccess:   hasAccess,
	}, nil
}

type entitlementAccessRow struct {
	FeatureKey string `json:"feature_key"`
	HasAccess  bool   `json:"has_access"`
}

type entitlementAccessResponse struct {
	Data []entitlementAccessRow `json:"data"`
}

func (c *Checker) entitlementHasAccess(ctx context.Context, customerID, featureKey string) (bool, error) {
	baseURL := resolveEntitlementAccessBaseURL(c.baseURL, c.apiKey)
	url := fmt.Sprintf(
		"%s/customers/%s/entitlement-access",
		strings.TrimSuffix(baseURL, "/"),
		customerID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return false, fmt.Errorf("entitlement-access request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return false, fmt.Errorf(
			"entitlement-access failed (%s) [%d]: %s",
			url,
			resp.StatusCode,
			strings.TrimSpace(string(body)),
		)
	}

	var payload entitlementAccessResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return false, fmt.Errorf("decoding entitlement-access response: %w", err)
	}
	for _, row := range payload.Data {
		if row.FeatureKey == featureKey {
			return row.HasAccess, nil
		}
	}
	return false, nil
}

func resolveEntitlementAccessBaseURL(baseURL, apiKey string) string {
	url := strings.TrimSpace(baseURL)
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, "/events")
	if isKonnectMeteringURL(url, apiKey) {
		if !strings.HasSuffix(url, "/openmeter") && konnectVersionSuffix.MatchString(url) {
			url = url + "/openmeter"
		}
		return url
	}
	return url
}

func isKonnectMeteringURL(url, apiKey string) bool {
	if strings.Contains(strings.ToLower(url), "konghq.com") {
		return true
	}
	key := strings.TrimSpace(apiKey)
	return strings.HasPrefix(key, "kpat_") || strings.HasPrefix(key, "spat_")
}

// ParseEntitlementAccess parses a Konnect entitlement-access JSON body.
func ParseEntitlementAccess(raw []byte, featureKey string) (bool, error) {
	var payload entitlementAccessResponse
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, err
	}
	for _, row := range payload.Data {
		if row.FeatureKey == featureKey {
			return row.HasAccess, nil
		}
	}
	return false, nil
}
