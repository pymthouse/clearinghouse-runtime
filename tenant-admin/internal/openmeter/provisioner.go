package openmeter

import (
	"context"
	"fmt"
	"strings"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
	"github.com/Kong/sdk-konnect-go/models/components"
	"github.com/Kong/sdk-konnect-go/models/operations"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/customerkey"
)

type Provisioner struct {
	sdk     *sdkkonnectgo.SDK
	planKey string
}

type MeterDefinition struct {
	Key           string
	Name          string
	Aggregation   components.Aggregation
	EventType     string
	ValueProperty *string
}

var defaultMeters = []MeterDefinition{
	{
		Key:           "network_fee_usd_micros",
		Name:          "Network Fee USD Micros",
		Aggregation:   components.AggregationSum,
		EventType:     "create_signed_ticket",
		ValueProperty: sdkkonnectgo.Pointer("$.network_fee_usd_micros"),
	},
	{
		Key:           "billable_usd_micros",
		Name:          "Billable USD Micros",
		Aggregation:   components.AggregationSum,
		EventType:     "create_signed_ticket",
		ValueProperty: sdkkonnectgo.Pointer("$.billable_usd_micros"),
	},
	{
		Key:         "signed_ticket_count",
		Name:        "Signed Ticket Count",
		Aggregation: components.AggregationCount,
		EventType:   "create_signed_ticket",
	},
}

const (
	defaultFeatureKey  = "network_spend"
	billableFeatureKey = "billable_spend"
)

type ProvisionInput struct {
	TenantID       string
	ClientID       string
	ExternalUserID string
	DisplayName    string
}

type ProvisionResult struct {
	CustomerKey    string `json:"customerKey"`
	CustomerID     string `json:"customerId"`
	SubscriptionID string `json:"subscriptionId"`
	PlanKey        string `json:"planKey"`
	Status         string `json:"status"`
	Created        struct {
		Customer     bool `json:"customer"`
		Subscription bool `json:"subscription"`
	} `json:"created"`
}

func NewProvisioner(baseURL, apiKey, planKey string) *Provisioner {
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
	return &Provisioner{
		sdk:     sdkkonnectgo.New(options...),
		planKey: strings.TrimSpace(planKey),
	}
}

func (p *Provisioner) Ensure(ctx context.Context, input ProvisionInput) (*ProvisionResult, error) {
	clientID := strings.TrimSpace(input.ClientID)
	externalUserID := strings.TrimSpace(input.ExternalUserID)
	tenantID := strings.TrimSpace(input.TenantID) // optional: label only, not part of the key
	if clientID == "" || externalUserID == "" {
		return nil, fmt.Errorf("clientId and externalUserId must be non-empty")
	}
	if p.planKey == "" {
		return nil, fmt.Errorf("plan key must be non-empty")
	}

	// Customer key == event subject == clientId:externalUserId (builder-sdk/pymthouse format).
	customerKey, err := customerkey.Build(clientID, externalUserID)
	if err != nil {
		return nil, err
	}
	displayName := strings.TrimSpace(input.DisplayName)
	if displayName == "" {
		displayName = customerKey
	}

	customerID, customerCreated, err := p.ensureCustomer(ctx, tenantID, clientID, externalUserID, customerKey, displayName)
	if err != nil {
		return nil, err
	}

	subscriptionID, status, subscriptionCreated, err := p.ensureSubscription(ctx, customerID)
	if err != nil {
		return nil, err
	}

	result := &ProvisionResult{
		CustomerKey:    customerKey,
		CustomerID:     customerID,
		SubscriptionID: subscriptionID,
		PlanKey:        p.planKey,
		Status:         status,
	}
	result.Created.Customer = customerCreated
	result.Created.Subscription = subscriptionCreated
	return result, nil
}

func (p *Provisioner) EnsureDefaultCatalog(ctx context.Context) error {
	meterIDs := make(map[string]string, len(defaultMeters))
	for _, definition := range defaultMeters {
		meterID, err := p.ensureMeter(ctx, definition)
		if err != nil {
			return err
		}
		meterIDs[definition.Key] = meterID
	}
	if _, err := p.ensureFeature(ctx, defaultFeatureKey, meterIDs["network_fee_usd_micros"]); err != nil {
		return err
	}
	if _, err := p.ensureFeature(ctx, billableFeatureKey, meterIDs["billable_usd_micros"]); err != nil {
		return err
	}
	if _, err := p.resolveActivePlan(ctx); err != nil {
		return err
	}
	return nil
}

func normalizeOpenMeterURL(baseURL string) string {
	url := strings.TrimSpace(baseURL)
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, "/events")
	url = strings.TrimSuffix(url, "/openmeter")
	url = strings.TrimSuffix(url, "/v3")
	return url
}

func (p *Provisioner) ensureMeter(ctx context.Context, definition MeterDefinition) (string, error) {
	if existingID, err := p.findMeterByKey(ctx, definition.Key); err != nil {
		return "", err
	} else if existingID != "" {
		return existingID, nil
	}

	response, err := p.sdk.Meters.CreateMeter(ctx, components.CreateMeterRequest{
		Name:          definition.Name,
		Key:           definition.Key,
		Aggregation:   definition.Aggregation,
		EventType:     definition.EventType,
		ValueProperty: definition.ValueProperty,
	})
	if err != nil {
		if existingID, findErr := p.findMeterByKey(ctx, definition.Key); findErr == nil && existingID != "" {
			return existingID, nil
		}
		return "", fmt.Errorf("create meter %s: %w", definition.Key, err)
	}
	if response.Meter == nil || strings.TrimSpace(response.Meter.ID) == "" {
		return "", fmt.Errorf("create meter %s: empty response", definition.Key)
	}
	return strings.TrimSpace(response.Meter.ID), nil
}

func (p *Provisioner) findMeterByKey(ctx context.Context, meterKey string) (string, error) {
	pageNumber := int64(1)
	pageSize := int64(100)
	targetKey := strings.TrimSpace(meterKey)

	for {
		response, err := p.sdk.Meters.ListMeters(ctx, operations.ListMetersRequest{
			Page: &components.PagePaginationQuery{
				Number: sdkkonnectgo.Pointer(pageNumber),
				Size:   sdkkonnectgo.Pointer(pageSize),
			},
		})
		if err != nil {
			return "", fmt.Errorf("list meters for key %s: %w", meterKey, err)
		}
		if response.MeterPagePaginatedResponse == nil {
			return "", nil
		}
		for _, meter := range response.MeterPagePaginatedResponse.Data {
			if strings.TrimSpace(meter.Key) == targetKey && strings.TrimSpace(meter.ID) != "" {
				return strings.TrimSpace(meter.ID), nil
			}
		}
		if len(response.MeterPagePaginatedResponse.Data) < int(pageSize) {
			return "", nil
		}
		pageNumber++
	}
}

func (p *Provisioner) ensureFeature(ctx context.Context, featureKey string, meterID string) (string, error) {
	if existingID, err := p.findFeatureByKey(ctx, featureKey); err != nil {
		return "", err
	} else if existingID != "" {
		return existingID, nil
	}
	response, err := p.sdk.OpenMeterFeatures.CreateFeature(ctx, components.CreateFeatureRequest{
		Name: "Network Spend",
		Key:  strings.TrimSpace(featureKey),
		Meter: &components.CreateFeatureRequestMeterReference{
			ID: strings.TrimSpace(meterID),
		},
	})
	if err != nil {
		if existingID, findErr := p.findFeatureByKey(ctx, featureKey); findErr == nil && existingID != "" {
			return existingID, nil
		}
		return "", fmt.Errorf("create feature %s: %w", featureKey, err)
	}
	if response.Feature == nil || strings.TrimSpace(response.Feature.ID) == "" {
		return "", fmt.Errorf("create feature %s: empty response", featureKey)
	}
	return strings.TrimSpace(response.Feature.ID), nil
}

func (p *Provisioner) findFeatureByKey(ctx context.Context, featureKey string) (string, error) {
	response, err := p.sdk.OpenMeterFeatures.ListFeatures(ctx, operations.ListFeaturesRequest{
		Page: &components.PagePaginationQuery{
			Number: sdkkonnectgo.Pointer(int64(1)),
			Size:   sdkkonnectgo.Pointer(int64(100)),
		},
		Filter: &components.ListFeatureParamsFilter{
			Key: &components.StringFieldFilter{
				Eq: sdkkonnectgo.Pointer(strings.TrimSpace(featureKey)),
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("list features for key %s: %w", featureKey, err)
	}
	if response.FeaturePagePaginatedResponse == nil {
		return "", nil
	}
	for _, feature := range response.FeaturePagePaginatedResponse.Data {
		if strings.TrimSpace(feature.Key) == strings.TrimSpace(featureKey) && strings.TrimSpace(feature.ID) != "" {
			return strings.TrimSpace(feature.ID), nil
		}
	}
	return "", nil
}

func (p *Provisioner) ensureCustomer(ctx context.Context, tenantID, clientID, externalUserID, customerKey, displayName string) (string, bool, error) {
	if existing, err := p.findCustomerByKey(ctx, customerKey); err != nil {
		return "", false, err
	} else if existing != "" {
		return existing, false, nil
	}

	response, err := p.sdk.OpenMeterCustomers.CreateCustomer(ctx, components.CreateCustomerRequest{
		Key:  customerKey,
		Name: displayName,
		Labels: map[string]string{
			"tenant_id":        strings.TrimSpace(tenantID),
			"client_id":        strings.TrimSpace(clientID),
			"external_user_id": strings.TrimSpace(externalUserID),
		},
		UsageAttribution: &components.UsageAttribution{
			SubjectKeys: []string{customerKey},
		},
	})
	if err != nil {
		if existing, findErr := p.findCustomerByKey(ctx, customerKey); findErr == nil && existing != "" {
			return existing, false, nil
		}
		return "", false, fmt.Errorf("creating customer %s: %w", customerKey, err)
	}
	if response.BillingCustomer == nil || response.BillingCustomer.ID == "" {
		return "", false, fmt.Errorf("creating customer %s: empty response", customerKey)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", false, fmt.Errorf("creating customer %s: status %d", customerKey, response.StatusCode)
	}
	return response.BillingCustomer.ID, true, nil
}

func (p *Provisioner) findCustomerByKey(ctx context.Context, customerKey string) (string, error) {
	response, err := p.sdk.OpenMeterCustomers.ListCustomers(ctx, operations.ListCustomersRequest{
		Filter: &components.ListCustomersParamsFilter{
			Key: &components.StringFieldFilter{
				Eq: sdkkonnectgo.Pointer(customerKey),
			},
		},
		Page: &components.PagePaginationQuery{
			Number: sdkkonnectgo.Pointer(int64(1)),
			Size:   sdkkonnectgo.Pointer(int64(100)),
		},
	})
	if err != nil {
		return "", fmt.Errorf("listing customers for key %s: %w", customerKey, err)
	}
	if response.CustomerPagePaginatedResponse != nil {
		for _, customer := range response.CustomerPagePaginatedResponse.Data {
			if customer.Key == customerKey && customer.ID != "" {
				return customer.ID, nil
			}
		}
	}

	getResponse, getErr := p.sdk.OpenMeterCustomers.GetCustomer(ctx, customerKey)
	if getErr == nil && getResponse.BillingCustomer != nil && getResponse.BillingCustomer.ID != "" {
		return getResponse.BillingCustomer.ID, nil
	}
	return "", nil
}

func subscriptionStatusActive(status components.BillingSubscriptionStatus) bool {
	switch status {
	case components.BillingSubscriptionStatusActive,
		components.BillingSubscriptionStatusScheduled:
		return true
	default:
		return false
	}
}

func planStatusUsable(status components.BillingPlanStatus) bool {
	switch status {
	case components.BillingPlanStatusActive,
		components.BillingPlanStatusScheduled:
		return true
	default:
		return false
	}
}

func (p *Provisioner) resolveActivePlan(ctx context.Context) (components.Plan, error) {
	const pageSize = int64(50)
	pageNumber := int64(1)
	var best *components.BillingPlan
	for {
		response, err := p.sdk.OpenMeterProductCatalog.ListPlans(ctx, operations.ListPlansRequest{
			Filter: &components.ListPlansParamsFilter{
				Key: &components.StringFieldFilter{
					Eq: sdkkonnectgo.Pointer(p.planKey),
				},
			},
			Page: &components.PagePaginationQuery{
				Number: sdkkonnectgo.Pointer(pageNumber),
				Size:   sdkkonnectgo.Pointer(pageSize),
			},
		})
		if err != nil {
			return components.Plan{}, fmt.Errorf("listing plans for key %s: %w", p.planKey, err)
		}
		if response.PlanPagePaginatedResponse == nil {
			break
		}
		for i := range response.PlanPagePaginatedResponse.Data {
			plan := response.PlanPagePaginatedResponse.Data[i]
			if plan.DeletedAt != nil || !planStatusUsable(plan.Status) || plan.ID == "" {
				continue
			}
			if best == nil || planVersion(plan) > planVersion(*best) {
				copyPlan := plan
				best = &copyPlan
			}
		}
		if len(response.PlanPagePaginatedResponse.Data) < int(pageSize) {
			break
		}
		pageNumber++
	}
	if best == nil {
		return components.Plan{}, fmt.Errorf("no active plan found for key %s", p.planKey)
	}
	return components.Plan{
		ID:      sdkkonnectgo.Pointer(best.ID),
		Key:     sdkkonnectgo.Pointer(p.planKey),
		Version: best.Version,
	}, nil
}

func planVersion(plan components.BillingPlan) int64 {
	if plan.Version == nil {
		return 0
	}
	return *plan.Version
}

func (p *Provisioner) ensureSubscription(ctx context.Context, customerID string) (string, string, bool, error) {
	const pageSize = int64(100)
	pageNumber := int64(1)
	for {
		customerFilter := components.CreateULIDFieldFilterStr(customerID)
		response, err := p.sdk.OpenMeterSubscriptions.ListSubscriptions(ctx, operations.ListSubscriptionsRequest{
			Filter: &components.ListSubscriptionsParamsFilter{
				CustomerID: &customerFilter,
				PlanKey: &components.StringFieldFilterExact{
					Eq: sdkkonnectgo.Pointer(p.planKey),
				},
			},
			Page: &components.PagePaginationQuery{
				Number: sdkkonnectgo.Pointer(pageNumber),
				Size:   sdkkonnectgo.Pointer(pageSize),
			},
		})
		if err != nil {
			return "", "", false, fmt.Errorf("listing subscriptions for customer %s: %w", customerID, err)
		}
		if response.SubscriptionPagePaginatedResponse != nil {
			for _, subscription := range response.SubscriptionPagePaginatedResponse.Data {
				if subscription.ID != "" && subscriptionStatusActive(subscription.Status) {
					return subscription.ID, string(subscription.Status), false, nil
				}
			}
			if len(response.SubscriptionPagePaginatedResponse.Data) < int(pageSize) {
				break
			}
		} else {
			break
		}
		pageNumber++
	}

	planRef, err := p.resolveActivePlan(ctx)
	if err != nil {
		return "", "", false, err
	}
	createResponse, err := p.sdk.OpenMeterSubscriptions.CreateSubscription(ctx, components.BillingSubscriptionCreate{
		Customer: components.BillingSubscriptionCreateCustomer{
			ID: sdkkonnectgo.Pointer(customerID),
		},
		Plan: planRef,
	})
	if err != nil {
		normalizedErr := strings.ToLower(err.Error())
		if strings.Contains(normalizedErr, "invalid billing setup") {
			return "", "pending_billing_setup", false, nil
		}
		return "", "", false, fmt.Errorf("creating subscription for customer %s plan %s: %w", customerID, p.planKey, err)
	}
	if createResponse.BillingSubscription == nil || createResponse.BillingSubscription.ID == "" {
		return "", "", false, fmt.Errorf("creating subscription for customer %s plan %s: empty response", customerID, p.planKey)
	}
	if createResponse.StatusCode < 200 || createResponse.StatusCode >= 300 {
		return "", "", false, fmt.Errorf("creating subscription for customer %s plan %s: status %d", customerID, p.planKey, createResponse.StatusCode)
	}
	return createResponse.BillingSubscription.ID, string(createResponse.BillingSubscription.Status), true, nil
}
