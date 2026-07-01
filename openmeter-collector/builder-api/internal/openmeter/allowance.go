package openmeter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type entitlementValue struct {
	HasAccess                 bool    `json:"hasAccess"`
	Balance                   float64 `json:"balance"`
	Usage                     float64 `json:"usage"`
	TotalAvailableGrantAmount float64 `json:"totalAvailableGrantAmount"`
}

type createGrantRequest struct {
	Amount     int64              `json:"amount"`
	Priority   int              `json:"priority"`
	EffectiveAt time.Time       `json:"effectiveAt"`
	Expiration grantExpiration   `json:"expiration"`
}

type grantExpiration struct {
	Duration string `json:"duration"`
	Count    int    `json:"count"`
}

// GetTrialCreditBalance reads the user-scoped trial allowance for a customer key.
func (c *Client) GetTrialCreditBalance(ctx context.Context, customerKey, featureKey string, defaultGrantMicros int64) (TrialCreditBalance, error) {
	featureKey = strings.TrimSpace(featureKey)
	if featureKey == "" {
		featureKey = "network_spend"
	}

	value, err := c.getEntitlementValue(ctx, customerKey, featureKey)
	if err != nil {
		return TrialCreditBalance{
			HasAccess:                false,
			BalanceUsdMicros:         "0",
			ConsumedUsdMicros:        "0",
			LifetimeGrantedUsdMicros: "0",
		}, nil
	}

	balance := int64(max(0, value.Balance))
	usage := int64(max(0, value.Usage))
	granted := int64(max(0, value.TotalAvailableGrantAmount))
	if granted == 0 {
		granted = balance + usage
	}
	if granted == 0 && defaultGrantMicros > 0 {
		granted = defaultGrantMicros
	}

	return TrialCreditBalance{
		HasAccess:                value.HasAccess && balance > 0,
		BalanceUsdMicros:         strconv.FormatInt(balance, 10),
		ConsumedUsdMicros:        strconv.FormatInt(usage, 10),
		LifetimeGrantedUsdMicros: strconv.FormatInt(granted, 10),
	}, nil
}

// GetTrialCreditBalanceWithFallback returns entitlement balance when available,
// otherwise falls back to the configured starter grant for Konnect-style deployments.
func (c *Client) GetTrialCreditBalanceWithFallback(ctx context.Context, customerKey, featureKey string, defaultGrantMicros int64) (TrialCreditBalance, error) {
	balance, err := c.GetTrialCreditBalance(ctx, customerKey, featureKey, defaultGrantMicros)
	if err != nil {
		return balance, err
	}
	if balance.HasAccess || defaultGrantMicros <= 0 {
		return balance, nil
	}

	if _, entErr := c.getEntitlementValue(ctx, customerKey, featureKey); entErr == nil {
		return balance, nil
	}

	defaultGrant := strconv.FormatInt(defaultGrantMicros, 10)
	return TrialCreditBalance{
		HasAccess:                true,
		BalanceUsdMicros:         defaultGrant,
		ConsumedUsdMicros:        "0",
		LifetimeGrantedUsdMicros: defaultGrant,
	}, nil
}

// EnsureTrialGrant creates a one-year trial grant when the customer has no allowance yet.
func (c *Client) EnsureTrialGrant(ctx context.Context, customerKey, featureKey string, amountMicros int64) error {
	if amountMicros <= 0 {
		return nil
	}
	featureKey = strings.TrimSpace(featureKey)
	if featureKey == "" {
		featureKey = "network_spend"
	}

	balance, err := c.GetTrialCreditBalance(ctx, customerKey, featureKey, amountMicros)
	if err != nil {
		return err
	}
	if balance.HasAccess {
		return nil
	}

	payload := createGrantRequest{
		Amount:     amountMicros,
		Priority:   1,
		EffectiveAt: time.Now().UTC(),
		Expiration: grantExpiration{Duration: "YEAR", Count: 1},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	path := fmt.Sprintf("%s/customers/%s/entitlements/%s/grants",
		c.baseURL,
		urlPathEscape(customerKey),
		urlPathEscape(featureKey),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("openmeter create grant: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("openmeter create grant %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (c *Client) getEntitlementValue(ctx context.Context, customerKey, featureKey string) (*entitlementValue, error) {
	path := fmt.Sprintf("%s/customers/%s/entitlements/%s/value",
		c.baseURL,
		urlPathEscape(customerKey),
		urlPathEscape(featureKey),
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openmeter entitlement value: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openmeter entitlement value %d: %s", resp.StatusCode, string(respBody))
	}

	var value entitlementValue
	if err := json.Unmarshal(respBody, &value); err != nil {
		return nil, err
	}
	return &value, nil
}

func urlPathEscape(value string) string {
	return strings.ReplaceAll(value, ":", "%3A")
}

func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
