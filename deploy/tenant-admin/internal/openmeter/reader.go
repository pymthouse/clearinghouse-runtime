package openmeter

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
	"github.com/Kong/sdk-konnect-go/models/components"
	"github.com/Kong/sdk-konnect-go/models/operations"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/billingidentity"
)

const (
	defaultTrialFeatureKey = "network_spend"
)

type UsageRow struct {
	ExternalUserID      string `json:"externalUserId"`
	RequestCount        int64  `json:"requestCount"`
	NetworkFeeUSDMicros int64  `json:"networkFeeUsdMicros"`
}

type UsageSummary struct {
	TenantID string `json:"tenantId"`
	ClientID string `json:"clientId"`
	Period   struct {
		Start string `json:"start"`
		End   string `json:"end"`
	} `json:"period"`
	Rows []UsageRow `json:"rows"`
}

type BalanceSummary struct {
	TenantID             string `json:"tenantId"`
	ClientID             string `json:"clientId"`
	ExternalUserID       string `json:"externalUserId"`
	FeatureKey           string `json:"featureKey"`
	HasAccess            bool   `json:"hasAccess"`
	BalanceUSDInMicros   string `json:"balanceUsdMicros"`
	ConsumedUSDInMicros  string `json:"consumedUsdMicros"`
	LifetimeUSDInMicros  string `json:"lifetimeGrantedUsdMicros"`
	RemainingUSDInMicros string `json:"remainingUsdMicros"`
}

func ReadUsage(
	ctx context.Context,
	baseURL string,
	apiKey string,
	tenantID string,
	clientID string,
	externalUserID string,
	startDate string,
	endDate string,
) (*UsageSummary, error) {
	trimmedTenantID := strings.TrimSpace(tenantID)
	trimmedClientID := strings.TrimSpace(clientID)
	trimmedExternalUserID := strings.TrimSpace(externalUserID)
	if trimmedTenantID == "" || trimmedClientID == "" {
		return nil, fmt.Errorf("tenantId and clientId are required")
	}

	from, to := defaultPeriod(startDate, endDate)
	subjectFilter, err := billingidentity.BuildCustomerKey(trimmedTenantID, trimmedClientID, trimmedExternalUserID)
	if err != nil {
		return nil, err
	}
	sdk := buildReaderSDK(baseURL, apiKey)

	feeRows, err := queryMeterRows(ctx, sdk, "network_fee_usd_micros", trimmedClientID, subjectFilter, from, to)
	if err != nil {
		return nil, err
	}
	countRows, err := queryMeterRows(ctx, sdk, "signed_ticket_count", trimmedClientID, subjectFilter, from, to)
	if err != nil {
		return nil, err
	}

	rowsByExternalUserID := map[string]UsageRow{}
	for _, row := range countRows {
		externalID := strings.TrimSpace(row.Dimensions["external_user_id"])
		if externalID == "" {
			externalID = trimmedExternalUserID
		}
		current := rowsByExternalUserID[externalID]
		current.ExternalUserID = externalID
		current.RequestCount += parseMeterValue(row.Value)
		rowsByExternalUserID[externalID] = current
	}
	for _, row := range feeRows {
		externalID := strings.TrimSpace(row.Dimensions["external_user_id"])
		if externalID == "" {
			externalID = trimmedExternalUserID
		}
		current := rowsByExternalUserID[externalID]
		current.ExternalUserID = externalID
		current.NetworkFeeUSDMicros += parseMeterValue(row.Value)
		rowsByExternalUserID[externalID] = current
	}

	resultRows := make([]UsageRow, 0, len(rowsByExternalUserID))
	for _, row := range rowsByExternalUserID {
		if trimmedExternalUserID != "" && row.ExternalUserID != trimmedExternalUserID {
			continue
		}
		resultRows = append(resultRows, row)
	}

	summary := &UsageSummary{
		TenantID: trimmedTenantID,
		ClientID: trimmedClientID,
		Rows:     resultRows,
	}
	summary.Period.Start = from.Format(time.RFC3339)
	summary.Period.End = to.Format(time.RFC3339)
	return summary, nil
}

func ReadBalance(
	ctx context.Context,
	baseURL string,
	apiKey string,
	tenantID string,
	clientID string,
	externalUserID string,
	featureKey string,
) (*BalanceSummary, error) {
	trimmedTenantID := strings.TrimSpace(tenantID)
	trimmedClientID := strings.TrimSpace(clientID)
	trimmedExternalUserID := strings.TrimSpace(externalUserID)
	trimmedFeatureKey := strings.TrimSpace(featureKey)
	if trimmedFeatureKey == "" {
		trimmedFeatureKey = defaultTrialFeatureKey
	}
	customerKey, err := billingidentity.BuildCustomerKey(trimmedTenantID, trimmedClientID, trimmedExternalUserID)
	if err != nil {
		return nil, err
	}
	sdk := buildReaderSDK(baseURL, apiKey)
	customerID, err := findCustomerIDByKey(ctx, sdk, customerKey)
	if err != nil {
		return nil, err
	}

	entitlementsResponse, err := sdk.OpenMeterEntitlements.ListCustomerEntitlementAccess(ctx, customerID)
	if err != nil {
		return nil, err
	}
	hasAccess := false
	if entitlementsResponse.ListCustomerEntitlementAccessResponseData != nil {
		for _, item := range entitlementsResponse.ListCustomerEntitlementAccessResponseData.Data {
			if strings.TrimSpace(item.FeatureKey) == trimmedFeatureKey {
				hasAccess = item.HasAccess
				break
			}
		}
	}

	return &BalanceSummary{
		TenantID:             trimmedTenantID,
		ClientID:             trimmedClientID,
		ExternalUserID:       trimmedExternalUserID,
		FeatureKey:           trimmedFeatureKey,
		HasAccess:            hasAccess,
		BalanceUSDInMicros:   "0",
		ConsumedUSDInMicros:  "0",
		LifetimeUSDInMicros:  "0",
		RemainingUSDInMicros: "0",
	}, nil
}

func buildReaderSDK(baseURL string, apiKey string) *sdkkonnectgo.SDK {
	url := normalizeOpenMeterURL(baseURL)
	options := []sdkkonnectgo.SDKOption{
		sdkkonnectgo.WithSecurity(components.Security{
			PersonalAccessToken: sdkkonnectgo.Pointer(strings.TrimSpace(apiKey)),
		}),
	}
	if url != "" {
		options = append(options, sdkkonnectgo.WithServerURL(url))
	} else {
		options = append(options, sdkkonnectgo.WithServerIndex(1))
	}
	return sdkkonnectgo.New(options...)
}

func queryMeterRows(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	meterKey string,
	clientID string,
	subject string,
	from time.Time,
	to time.Time,
) ([]components.MeterQueryRow, error) {
	meterID, err := findMeterIDByKey(ctx, sdk, meterKey)
	if err != nil {
		return nil, err
	}
	subjectFilter := strings.TrimSpace(subject)
	result, err := sdk.Meters.QueryMeter(
		ctx,
		meterID,
		components.MeterQueryRequest{
			From:              &from,
			To:                &to,
			GroupByDimensions: []string{"client_id", "external_user_id"},
			Filters: &components.Filters{
				Dimensions: map[string]components.QueryFilterStringMapItem{
					"client_id": {
						Eq: sdkkonnectgo.Pointer(strings.TrimSpace(clientID)),
					},
					"subject": {
						Eq: &subjectFilter,
					},
				},
			},
		},
	)
	if err != nil {
		return nil, err
	}
	if result.MeterQueryResult == nil {
		return []components.MeterQueryRow{}, nil
	}
	return result.MeterQueryResult.Data, nil
}

func findCustomerIDByKey(ctx context.Context, sdk *sdkkonnectgo.SDK, customerKey string) (string, error) {
	response, err := sdk.OpenMeterCustomers.ListCustomers(ctx, operations.ListCustomersRequest{
		Filter: &components.ListCustomersParamsFilter{
			Key: &components.StringFieldFilter{
				Eq: sdkkonnectgo.Pointer(strings.TrimSpace(customerKey)),
			},
		},
		Page: &components.PagePaginationQuery{
			Number: sdkkonnectgo.Pointer(int64(1)),
			Size:   sdkkonnectgo.Pointer(int64(10)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("list customer for key %s: %w", customerKey, err)
	}
	if response.CustomerPagePaginatedResponse != nil {
		for _, customer := range response.CustomerPagePaginatedResponse.Data {
			if customer.Key == customerKey && strings.TrimSpace(customer.ID) != "" {
				return strings.TrimSpace(customer.ID), nil
			}
		}
	}
	return "", fmt.Errorf("customer not found for key %s", customerKey)
}

func findMeterIDByKey(ctx context.Context, sdk *sdkkonnectgo.SDK, meterKey string) (string, error) {
	response, err := sdk.Meters.ListMeters(ctx, operations.ListMetersRequest{
		Page: &components.PagePaginationQuery{
			Number: sdkkonnectgo.Pointer(int64(1)),
			Size:   sdkkonnectgo.Pointer(int64(100)),
		},
	})
	if err != nil {
		return "", err
	}
	if response.MeterPagePaginatedResponse == nil {
		return "", fmt.Errorf("meter %s not found", meterKey)
	}
	for _, meter := range response.MeterPagePaginatedResponse.Data {
		if strings.TrimSpace(meter.Key) == strings.TrimSpace(meterKey) && strings.TrimSpace(meter.ID) != "" {
			return strings.TrimSpace(meter.ID), nil
		}
	}
	return "", fmt.Errorf("meter %s not found", meterKey)
}

func parseMeterValue(raw string) int64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return 0
	}
	return int64(value)
}

func defaultPeriod(startDate string, endDate string) (time.Time, time.Time) {
	now := time.Now().UTC()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0).Add(-time.Millisecond)
	if parsedFrom, err := time.Parse(time.RFC3339, strings.TrimSpace(startDate)); err == nil {
		from = parsedFrom
	}
	if parsedTo, err := time.Parse(time.RFC3339, strings.TrimSpace(endDate)); err == nil {
		to = parsedTo
	}
	return from, to
}
