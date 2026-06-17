package admin

import (
	"context"
	"fmt"

	"github.com/livepeer/clearinghouse/internal/meters"
	"github.com/livepeer/clearinghouse/internal/pricing"
)

type BootstrapResult struct {
	Meters   []EnsureResult[Meter]
	Features []EnsureResult[Feature]
	Plan     *EnsureResult[Plan]

	PlanSkippedReason string
	Prune             *PruneResult

	PlanKey             string
	BillableFeatureKey  string
	TrialIncludedMicros string
}

func ensureMeter(ctx context.Context, admin OpenMeterAdmin, input MeterInput) (*EnsureResult[Meter], error) {
	existing, err := admin.ListMeters(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range existing {
		if m.Key == input.Key {
			var warnings []string
			_, hasPipeline := m.Dimensions["pipeline"]
			_, hasModelID := m.Dimensions["model_id"]
			if !hasPipeline || !hasModelID {
				warnings = append(warnings, fmt.Sprintf("meter %s missing pipeline/model_id dimensions — recreate manually if needed", input.Key))
			}
			return &EnsureResult[Meter]{Resource: m, Created: false, Warnings: warnings}, nil
		}
	}
	created, err := admin.CreateMeter(ctx, input)
	if err != nil {
		return nil, err
	}
	return &EnsureResult[Meter]{Resource: *created, Created: true}, nil
}

func ensureFeature(ctx context.Context, admin OpenMeterAdmin, input FeatureInput) (*EnsureResult[Feature], error) {
	features, err := admin.ListFeatures(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range features {
		if f.Key == input.Key {
			return &EnsureResult[Feature]{Resource: f, Created: false}, nil
		}
	}
	created, err := admin.CreateFeature(ctx, input)
	if err != nil {
		return &EnsureResult[Feature]{
			Resource: Feature{Key: input.Key, Name: input.Name, MeterID: input.MeterID},
			Created:  false,
			Warnings: []string{fmt.Sprintf("feature bootstrap skipped for %s: %s", input.Key, err)},
		}, nil
	}
	return &EnsureResult[Feature]{Resource: *created, Created: true}, nil
}

func BootstrapCatalog(
	ctx context.Context,
	admin OpenMeterAdmin,
	meterCfg *meters.Config,
	pricingCfg *pricing.Config,
	trialFeatureKey string,
	opts BootstrapOptions,
) (*BootstrapResult, error) {
	if err := admin.WaitForHealthy(ctx); err != nil {
		return nil, err
	}

	result := &BootstrapResult{
		PlanKey:             pricingCfg.DefaultPlanKey,
		BillableFeatureKey:  pricingCfg.BillableFeatureKey,
		TrialIncludedMicros: pricingCfg.DefaultTrialIncludedUsdMicros,
	}

	if opts.Prune {
		pruneResult, err := PruneCatalog(ctx, admin, meterCfg, pricingCfg, trialFeatureKey)
		if err != nil {
			return nil, err
		}
		result.Prune = pruneResult
	}

	defs := meterCfg.KonnectMeterDefinitions()

	var ensuredMeters []EnsureResult[Meter]
	for _, d := range defs {
		meterResult, err := ensureMeter(ctx, admin, MeterInput{
			Key:           d.Key,
			Name:          d.Name,
			Description:   d.Description,
			EventType:     d.EventType,
			Aggregation:   d.Aggregation,
			ValueProperty: d.ValueProperty,
			Dimensions:    d.Dimensions,
		})
		if err != nil {
			return nil, err
		}
		ensuredMeters = append(ensuredMeters, *meterResult)
	}

	// Re-fetch meters to get IDs
	allMeters, err := admin.ListMeters(ctx)
	if err != nil {
		return nil, err
	}
	meterByKey := make(map[string]Meter)
	for _, m := range allMeters {
		meterByKey[m.Key] = m
	}

	networkFeeMeter, ok := meterByKey[meterCfg.NetworkFeeUsdMicrosMeter]
	if !ok {
		return nil, fmt.Errorf("meter missing after bootstrap: %s", meterCfg.NetworkFeeUsdMicrosMeter)
	}
	billableMeter, ok := meterByKey[meterCfg.BillableUsdMicrosMeter]
	if !ok {
		return nil, fmt.Errorf("meter missing after bootstrap: %s", meterCfg.BillableUsdMicrosMeter)
	}

	if trialFeatureKey == "" {
		trialFeatureKey = meterCfg.DefaultTrialFeatureKey
	}

	var ensuredFeatures []EnsureResult[Feature]

	nfResult, err := ensureFeature(ctx, admin, FeatureInput{
		Key:     trialFeatureKey,
		Name:    "Network spend",
		MeterID: networkFeeMeter.ID,
	})
	if err != nil {
		return nil, err
	}
	ensuredFeatures = append(ensuredFeatures, *nfResult)

	bfResult, err := ensureFeature(ctx, admin, FeatureInput{
		Key:     pricingCfg.BillableFeatureKey,
		Name:    "Billable spend",
		MeterID: billableMeter.ID,
	})
	if err != nil {
		return nil, err
	}
	ensuredFeatures = append(ensuredFeatures, *bfResult)

	result.Meters = ensuredMeters
	result.Features = ensuredFeatures

	// Ensure plan
	plans, err := admin.ListPlans(ctx)
	if err != nil {
		result.PlanSkippedReason = err.Error()
		return result, nil
	}
	for _, p := range plans {
		if p.Key == pricingCfg.DefaultPlanKey {
			result.Plan = &EnsureResult[Plan]{Resource: p, Created: false}
			return result, nil
		}
	}

	plan, err := admin.EnsurePlan(ctx, PlanInput{
		Key:              pricingCfg.DefaultPlanKey,
		Name:             "Clearinghouse Default Pay-Per-Use",
		FeatureKey:       pricingCfg.BillableFeatureKey,
		FeatureName:      "Billable spend",
		BillableMetterID: billableMeter.ID,
		UnitAmount:       pricingCfg.UnitPriceUsdPerBillableMicro,
		Currency:         "USD",
		BillingCadence:   "P1M",
	})
	if err != nil {
		result.PlanSkippedReason = err.Error()
		return result, nil
	}
	result.Plan = &EnsureResult[Plan]{Resource: *plan, Created: true}
	return result, nil
}
