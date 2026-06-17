package admin

import (
	"context"
	"fmt"
	"time"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
	"github.com/Kong/sdk-konnect-go/models/components"
	"github.com/Kong/sdk-konnect-go/models/operations"
)

type KonnectAdmin struct {
	sdk *sdkkonnectgo.SDK
}

type KonnectAdminConfig struct {
	APIKey  string
	BaseURL string
}

func NewKonnectAdmin(cfg KonnectAdminConfig) *KonnectAdmin {
	opts := []sdkkonnectgo.SDKOption{
		sdkkonnectgo.WithSecurity(components.Security{
			PersonalAccessToken: sdkkonnectgo.Pointer(cfg.APIKey),
		}),
	}
	if cfg.BaseURL != "" {
		opts = append(opts, sdkkonnectgo.WithServerURL(cfg.BaseURL))
	} else {
		opts = append(opts, sdkkonnectgo.WithServerIndex(1)) // us.api.konghq.com
	}
	return &KonnectAdmin{sdk: sdkkonnectgo.New(opts...)}
}

func NewKonnectAdminWithSDK(sdk *sdkkonnectgo.SDK) *KonnectAdmin {
	return &KonnectAdmin{sdk: sdk}
}

func (k *KonnectAdmin) WaitForHealthy(ctx context.Context) error {
	const maxAttempts = 15
	const delay = 2 * time.Second

	for i := range maxAttempts {
		res, err := k.sdk.Meters.ListMeters(ctx, operations.ListMetersRequest{
			Page: &components.PagePaginationQuery{
				Number: sdkkonnectgo.Pointer(int64(1)),
				Size:   sdkkonnectgo.Pointer(int64(1)),
			},
		})
		if err == nil && res.StatusCode >= 200 && res.StatusCode < 300 {
			return nil
		}
		if i < maxAttempts-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}
	return NewBackendUnreachableError("Konnect Metering & Billing not reachable")
}

func (k *KonnectAdmin) ListMeters(ctx context.Context) ([]Meter, error) {
	res, err := k.sdk.Meters.ListMeters(ctx, operations.ListMetersRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing meters: %w", err)
	}
	var out []Meter
	if res.MeterPagePaginatedResponse != nil {
		for _, m := range res.MeterPagePaginatedResponse.Data {
			out = append(out, Meter{
				ID:         m.ID,
				Key:        m.Key,
				Name:       m.Name,
				Dimensions: m.Dimensions,
			})
		}
	}
	return out, nil
}

func (k *KonnectAdmin) CreateMeter(ctx context.Context, input MeterInput) (*Meter, error) {
	req := components.CreateMeterRequest{
		Key:           input.Key,
		Name:          input.Name,
		Description:   sdkkonnectgo.Pointer(input.Description),
		EventType:     input.EventType,
		Aggregation:   components.Aggregation(input.Aggregation),
		ValueProperty: nilIfEmpty(input.ValueProperty),
		Dimensions:    input.Dimensions,
	}
	res, err := k.sdk.Meters.CreateMeter(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating meter %s: %w", input.Key, err)
	}
	if res.Meter == nil {
		return nil, fmt.Errorf("creating meter %s: empty response", input.Key)
	}
	return &Meter{
		ID:         res.Meter.ID,
		Key:        res.Meter.Key,
		Name:       res.Meter.Name,
		Dimensions: res.Meter.Dimensions,
	}, nil
}

func (k *KonnectAdmin) ListFeatures(ctx context.Context) ([]Feature, error) {
	res, err := k.sdk.OpenMeterFeatures.ListFeatures(ctx, operations.ListFeaturesRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing features: %w", err)
	}
	var out []Feature
	if res.FeaturePagePaginatedResponse != nil {
		for _, f := range res.FeaturePagePaginatedResponse.Data {
			feat := Feature{ID: f.ID, Key: f.Key, Name: f.Name}
			if f.Meter != nil {
				feat.MeterID = f.Meter.ID
			}
			out = append(out, feat)
		}
	}
	return out, nil
}

func (k *KonnectAdmin) CreateFeature(ctx context.Context, input FeatureInput) (*Feature, error) {
	req := components.CreateFeatureRequest{
		Key:  input.Key,
		Name: input.Name,
	}
	if input.MeterID != "" {
		req.Meter = &components.CreateFeatureRequestMeterReference{ID: input.MeterID}
	}
	res, err := k.sdk.OpenMeterFeatures.CreateFeature(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("creating feature %s: %w", input.Key, err)
	}
	if res.Feature == nil {
		return nil, fmt.Errorf("creating feature %s: empty response", input.Key)
	}
	feat := &Feature{ID: res.Feature.ID, Key: res.Feature.Key, Name: res.Feature.Name}
	if res.Feature.Meter != nil {
		feat.MeterID = res.Feature.Meter.ID
	}
	return feat, nil
}

func (k *KonnectAdmin) ListPlans(ctx context.Context) ([]Plan, error) {
	res, err := k.sdk.OpenMeterProductCatalog.ListPlans(ctx, operations.ListPlansRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing plans: %w", err)
	}
	var out []Plan
	if res.PlanPagePaginatedResponse != nil {
		for _, p := range res.PlanPagePaginatedResponse.Data {
			out = append(out, Plan{
				ID:     p.ID,
				Key:    p.Key,
				Name:   p.Name,
				Status: string(p.Status),
			})
		}
	}
	return out, nil
}

func (k *KonnectAdmin) EnsurePlan(ctx context.Context, input PlanInput) (*Plan, error) {
	// Resolve feature ID
	features, err := k.ListFeatures(ctx)
	if err != nil {
		return nil, err
	}
	var featureID string
	for _, f := range features {
		if f.Key == input.FeatureKey {
			featureID = f.ID
			break
		}
	}
	if featureID == "" {
		created, err := k.CreateFeature(ctx, FeatureInput{
			Key:     input.FeatureKey,
			Name:    input.FeatureName,
			MeterID: input.BillableMetterID,
		})
		if err != nil {
			return nil, err
		}
		featureID = created.ID
	}

	// Pure pay-per-use rate card: no included units / discounts. Trial credit is
	// granted once per customer as an entitlement at provision time, not baked
	// into the plan (a plan-level discount recurs every billing period).
	rateCard := components.BillingRateCard{
		Key:            input.FeatureKey,
		Name:           "Billable usage",
		Feature:        &components.FeatureReference{ID: featureID},
		BillingCadence: sdkkonnectgo.Pointer("P1M"),
		PaymentTerm:    components.PaymentTermInArrears.ToPointer(),
		Price: components.CreatePriceUnit(components.BillingPriceUnit{
			Amount: input.UnitAmount,
		}),
	}

	currency := input.Currency
	if currency == "" {
		currency = "USD"
	}
	cadence := input.BillingCadence
	if cadence == "" {
		cadence = "P1M"
	}

	createRes, err := k.sdk.OpenMeterProductCatalog.CreatePlan(ctx, components.CreatePlanRequest{
		Key:              input.Key,
		Name:             input.Name,
		Currency:         currency,
		BillingCadence:   cadence,
		ProRatingEnabled: sdkkonnectgo.Pointer(true),
		Phases: []components.BillingPlanPhase{
			{
				Key:       "default",
				Name:      "Default",
				RateCards: []components.BillingRateCard{rateCard},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating plan %s: %w", input.Key, err)
	}
	if createRes.BillingPlan == nil {
		return nil, fmt.Errorf("creating plan %s: empty response", input.Key)
	}

	plan := createRes.BillingPlan
	if plan.Status == components.BillingPlanStatusDraft {
		_, err := k.sdk.OpenMeterProductCatalog.PublishPlan(ctx, plan.ID)
		if err != nil {
			return nil, fmt.Errorf("publishing plan %s: %w", input.Key, err)
		}
	}

	return &Plan{
		ID:   plan.ID,
		Key:  input.Key,
		Name: input.Name,
	}, nil
}

func (k *KonnectAdmin) DeleteMeter(ctx context.Context, id string) error {
	_, err := k.sdk.Meters.DeleteMeter(ctx, id)
	if err != nil {
		return fmt.Errorf("deleting meter %s: %w", id, err)
	}
	return nil
}

func (k *KonnectAdmin) DeleteFeature(ctx context.Context, id string) error {
	_, err := k.sdk.OpenMeterFeatures.DeleteFeature(ctx, id)
	if err != nil {
		return fmt.Errorf("deleting feature %s: %w", id, err)
	}
	return nil
}

func (k *KonnectAdmin) DeletePlan(ctx context.Context, id string) error {
	_, err := k.sdk.OpenMeterProductCatalog.DeletePlan(ctx, id)
	if err != nil {
		_, archiveErr := k.sdk.OpenMeterProductCatalog.ArchivePlan(ctx, id)
		if archiveErr != nil {
			return fmt.Errorf("deleting plan %s: %w (archive: %v)", id, err, archiveErr)
		}
	}
	return nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
