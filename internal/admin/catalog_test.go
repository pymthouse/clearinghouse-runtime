package admin

import (
	"context"
	"testing"

	"github.com/livepeer/clearinghouse/internal/meters"
	"github.com/livepeer/clearinghouse/internal/pricing"
)

type mockAdmin struct {
	meters   []Meter
	features []Feature
	plans    []Plan
}

func (m *mockAdmin) WaitForHealthy(_ context.Context) error { return nil }

func (m *mockAdmin) ListMeters(_ context.Context) ([]Meter, error) {
	return m.meters, nil
}

func (m *mockAdmin) CreateMeter(_ context.Context, input MeterInput) (*Meter, error) {
	meter := Meter{
		ID:         "meter-" + input.Key,
		Key:        input.Key,
		Name:       input.Name,
		Dimensions: input.Dimensions,
	}
	m.meters = append(m.meters, meter)
	return &meter, nil
}

func (m *mockAdmin) DeleteMeter(_ context.Context, id string) error {
	for i, meter := range m.meters {
		if meter.ID == id {
			m.meters = append(m.meters[:i], m.meters[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockAdmin) ListFeatures(_ context.Context) ([]Feature, error) {
	return m.features, nil
}

func (m *mockAdmin) CreateFeature(_ context.Context, input FeatureInput) (*Feature, error) {
	feat := Feature{
		ID:      "feat-" + input.Key,
		Key:     input.Key,
		Name:    input.Name,
		MeterID: input.MeterID,
	}
	m.features = append(m.features, feat)
	return &feat, nil
}

func (m *mockAdmin) DeleteFeature(_ context.Context, id string) error {
	for i, feat := range m.features {
		if feat.ID == id {
			m.features = append(m.features[:i], m.features[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockAdmin) ListPlans(_ context.Context) ([]Plan, error) {
	return m.plans, nil
}

func (m *mockAdmin) EnsurePlan(_ context.Context, input PlanInput) (*Plan, error) {
	plan := Plan{
		ID:   "plan-" + input.Key,
		Key:  input.Key,
		Name: input.Name,
	}
	m.plans = append(m.plans, plan)
	return &plan, nil
}

func (m *mockAdmin) DeletePlan(_ context.Context, id string) error {
	for i, plan := range m.plans {
		if plan.ID == id {
			m.plans = append(m.plans[:i], m.plans[i+1:]...)
			return nil
		}
	}
	return nil
}

func testMeterCfg() *meters.Config {
	meterCfg := &meters.Config{
		CreateSignedTicketEventType: "create_signed_ticket",
		NetworkFeeUsdMicrosMeter:    "network_fee_usd_micros",
		BillableUsdMicrosMeter:      "billable_usd_micros",
		SignedTicketCountMeter:      "signed_ticket_count",
		DefaultTrialFeatureKey:      "network_spend",
		DefaultBillableFeatureKey:   "billable_spend",
		Dimensions: map[string]string{
			"client_id":        "$.client_id",
			"pipeline":         "$.pipeline",
			"model_id":         "$.model_id",
			"external_user_id": "$.external_user_id",
		},
	}
	meterCfg.Meters.NetworkFeeUsdMicros.KonnectName = "Network fee"
	meterCfg.Meters.NetworkFeeUsdMicros.ValueProperty = "$.network_fee_usd_micros"
	meterCfg.Meters.BillableUsdMicros.KonnectName = "Billable"
	meterCfg.Meters.BillableUsdMicros.ValueProperty = "$.billable_usd_micros"
	meterCfg.Meters.SignedTicketCount.KonnectName = "Ticket count"
	return meterCfg
}

func testPricingCfg() *pricing.Config {
	return &pricing.Config{
		DefaultPlanKey:                "clearinghouse_default_ppu",
		BillableFeatureKey:            "billable_spend",
		BillableMeterKey:              "billable_usd_micros",
		UnitPriceUsdPerBillableMicro:  "0.000001",
		DefaultTrialIncludedUsdMicros: "5000000",
	}
}

func TestBootstrapCatalogCreatesAll(t *testing.T) {
	mock := &mockAdmin{}

	result, err := BootstrapCatalog(context.Background(), mock, testMeterCfg(), testPricingCfg(), "", BootstrapOptions{})
	if err != nil {
		t.Fatalf("BootstrapCatalog: %v", err)
	}

	if len(result.Meters) != 3 {
		t.Errorf("expected 3 meters, got %d", len(result.Meters))
	}
	for _, m := range result.Meters {
		if !m.Created {
			t.Errorf("meter %s should have been created", m.Resource.Key)
		}
	}

	if len(result.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(result.Features))
	}
	for _, f := range result.Features {
		if !f.Created {
			t.Errorf("feature %s should have been created", f.Resource.Key)
		}
	}

	if result.Plan == nil {
		t.Fatal("expected plan to be created")
	}
	if !result.Plan.Created {
		t.Error("plan should have been created")
	}
	if result.Plan.Resource.Key != "clearinghouse_default_ppu" {
		t.Errorf("plan key = %s, want clearinghouse_default_ppu", result.Plan.Resource.Key)
	}
}

func TestBootstrapCatalogIdempotent(t *testing.T) {
	mock := &mockAdmin{
		meters: []Meter{
			{ID: "m1", Key: "network_fee_usd_micros", Dimensions: testMeterCfg().Dimensions},
			{ID: "m2", Key: "billable_usd_micros", Dimensions: testMeterCfg().Dimensions},
			{ID: "m3", Key: "signed_ticket_count", Dimensions: testMeterCfg().Dimensions},
		},
		features: []Feature{
			{ID: "f1", Key: "network_spend", MeterID: "m1"},
			{ID: "f2", Key: "billable_spend", MeterID: "m2"},
		},
		plans: []Plan{
			{ID: "p1", Key: "clearinghouse_default_ppu"},
		},
	}

	result, err := BootstrapCatalog(context.Background(), mock, testMeterCfg(), testPricingCfg(), "", BootstrapOptions{})
	if err != nil {
		t.Fatalf("BootstrapCatalog: %v", err)
	}

	for _, m := range result.Meters {
		if m.Created {
			t.Errorf("meter %s should NOT have been created (idempotent)", m.Resource.Key)
		}
	}
	for _, f := range result.Features {
		if f.Created {
			t.Errorf("feature %s should NOT have been created (idempotent)", f.Resource.Key)
		}
	}
	if result.Plan == nil || result.Plan.Created {
		t.Error("plan should exist and NOT have been created (idempotent)")
	}
}

func TestPruneCatalogRemovesExtras(t *testing.T) {
	mock := &mockAdmin{
		meters: []Meter{
			{ID: "m1", Key: "network_fee_usd_micros", Dimensions: testMeterCfg().Dimensions},
			{ID: "m-old", Key: "legacy_meter", Dimensions: map[string]string{"x": "y"}},
		},
		features: []Feature{
			{ID: "f1", Key: "network_spend", MeterID: "m1"},
			{ID: "f-old", Key: "legacy_feature", MeterID: "m-old"},
		},
		plans: []Plan{
			{ID: "p1", Key: "clearinghouse_default_ppu"},
			{ID: "p-old", Key: "legacy_plan"},
		},
	}

	result, err := PruneCatalog(context.Background(), mock, testMeterCfg(), testPricingCfg(), "")
	if err != nil {
		t.Fatalf("PruneCatalog: %v", err)
	}

	if len(result.DeletedMeters) != 1 || result.DeletedMeters[0] != "legacy_meter" {
		t.Errorf("DeletedMeters = %v", result.DeletedMeters)
	}
	if len(result.DeletedFeatures) != 1 || result.DeletedFeatures[0] != "legacy_feature" {
		t.Errorf("DeletedFeatures = %v", result.DeletedFeatures)
	}
	if len(result.DeletedPlans) != 1 || result.DeletedPlans[0] != "legacy_plan" {
		t.Errorf("DeletedPlans = %v", result.DeletedPlans)
	}
}

func TestPruneCatalogRemovesMismatchedMeter(t *testing.T) {
	mock := &mockAdmin{
		meters: []Meter{
			{
				ID:  "m1",
				Key: "network_fee_usd_micros",
				Dimensions: map[string]string{
					"client_id": "$.client_id",
				},
			},
		},
	}

	result, err := PruneCatalog(context.Background(), mock, testMeterCfg(), testPricingCfg(), "")
	if err != nil {
		t.Fatalf("PruneCatalog: %v", err)
	}
	if len(result.DeletedMeters) != 1 {
		t.Fatalf("expected mismatched meter deleted, got %v", result.DeletedMeters)
	}
}

func TestBootstrapCatalogPruneRecreatesMeter(t *testing.T) {
	mock := &mockAdmin{
		meters: []Meter{
			{
				ID:  "m1",
				Key: "network_fee_usd_micros",
				Dimensions: map[string]string{
					"client_id": "$.client_id",
				},
			},
		},
	}

	result, err := BootstrapCatalog(context.Background(), mock, testMeterCfg(), testPricingCfg(), "", BootstrapOptions{Prune: true})
	if err != nil {
		t.Fatalf("BootstrapCatalog: %v", err)
	}
	if result.Prune == nil || len(result.Prune.DeletedMeters) == 0 {
		t.Fatal("expected prune to delete mismatched meter")
	}
	created := false
	for _, m := range result.Meters {
		if m.Resource.Key == "network_fee_usd_micros" && m.Created {
			created = true
		}
	}
	if !created {
		t.Error("expected network_fee_usd_micros to be recreated after prune")
	}
}
