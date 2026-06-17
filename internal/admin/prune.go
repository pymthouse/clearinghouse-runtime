package admin

import (
	"context"
	"fmt"

	"github.com/livepeer/clearinghouse/internal/meters"
	"github.com/livepeer/clearinghouse/internal/pricing"
)

type catalogExpectations struct {
	meters    map[string]MeterInput
	features  map[string]string // feature key -> meter key
	planKeys  map[string]struct{}
}

func buildCatalogExpectations(
	meterCfg *meters.Config,
	pricingCfg *pricing.Config,
	trialFeatureKey string,
) catalogExpectations {
	if trialFeatureKey == "" {
		trialFeatureKey = meterCfg.DefaultTrialFeatureKey
	}

	meterInputs := make(map[string]MeterInput)
	for _, d := range meterCfg.KonnectMeterDefinitions() {
		meterInputs[d.Key] = MeterInput{
			Key:           d.Key,
			Name:          d.Name,
			Description:   d.Description,
			EventType:     d.EventType,
			Aggregation:   d.Aggregation,
			ValueProperty: d.ValueProperty,
			Dimensions:    d.Dimensions,
		}
	}

	planKeys := map[string]struct{}{
		pricingCfg.DefaultPlanKey: {},
	}

	return catalogExpectations{
		meters: meterInputs,
		features: map[string]string{
			trialFeatureKey:            meterCfg.NetworkFeeUsdMicrosMeter,
			pricingCfg.BillableFeatureKey: meterCfg.BillableUsdMicrosMeter,
		},
		planKeys: planKeys,
	}
}

func dimensionsMatch(want, have map[string]string) bool {
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

func meterMatchesConfig(m Meter, expected MeterInput) bool {
	return dimensionsMatch(expected.Dimensions, m.Dimensions)
}

// PruneCatalog removes meters, features, and plans that are not defined in config,
// and removes configured meters/features whose shape no longer matches config.
func PruneCatalog(
	ctx context.Context,
	admin OpenMeterAdmin,
	meterCfg *meters.Config,
	pricingCfg *pricing.Config,
	trialFeatureKey string,
) (*PruneResult, error) {
	expect := buildCatalogExpectations(meterCfg, pricingCfg, trialFeatureKey)
	result := &PruneResult{}

	// Plans first (depend on features).
	plans, err := admin.ListPlans(ctx)
	if err != nil {
		return nil, err
	}
	for _, p := range plans {
		if _, ok := expect.planKeys[p.Key]; ok {
			continue
		}
		if err := admin.DeletePlan(ctx, p.ID); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("plan %s (%s): %v", p.Key, p.ID, err))
			continue
		}
		result.DeletedPlans = append(result.DeletedPlans, p.Key)
	}

	// Features before meters (features reference meters).
	allMeters, err := admin.ListMeters(ctx)
	if err != nil {
		return nil, err
	}
	meterIDByKey := make(map[string]string, len(allMeters))
	for _, m := range allMeters {
		meterIDByKey[m.Key] = m.ID
	}

	features, err := admin.ListFeatures(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range features {
		expectedMeterKey, configured := expect.features[f.Key]
		if !configured {
			if err := admin.DeleteFeature(ctx, f.ID); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("feature %s: %v", f.Key, err))
				continue
			}
			result.DeletedFeatures = append(result.DeletedFeatures, f.Key)
			continue
		}
		expectedMeterID := meterIDByKey[expectedMeterKey]
		if expectedMeterID != "" && f.MeterID != expectedMeterID {
			if err := admin.DeleteFeature(ctx, f.ID); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("feature %s (wrong meter): %v", f.Key, err))
				continue
			}
			result.DeletedFeatures = append(result.DeletedFeatures, f.Key)
		}
	}

	// Meters last.
	allMeters, err = admin.ListMeters(ctx)
	if err != nil {
		return nil, err
	}
	for _, m := range allMeters {
		expected, configured := expect.meters[m.Key]
		if !configured {
			if err := admin.DeleteMeter(ctx, m.ID); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("meter %s: %v", m.Key, err))
				continue
			}
			result.DeletedMeters = append(result.DeletedMeters, m.Key)
			continue
		}
		if !meterMatchesConfig(m, expected) {
			if err := admin.DeleteMeter(ctx, m.ID); err != nil {
				result.Warnings = append(result.Warnings, fmt.Sprintf("meter %s (config mismatch): %v", m.Key, err))
				continue
			}
			result.DeletedMeters = append(result.DeletedMeters, m.Key)
		}
	}

	return result, nil
}
