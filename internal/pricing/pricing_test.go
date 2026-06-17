package pricing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	os.WriteFile(path, []byte(`{
		"defaultTrialIncludedUsdMicros": "5000000",
		"defaultPlanKey": "clearinghouse_default_ppu",
		"billableFeatureKey": "billable_spend",
		"billableMeterKey": "billable_usd_micros",
		"unitPriceUsdPerBillableMicro": "0.000001"
	}`), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DefaultPlanKey != "clearinghouse_default_ppu" {
		t.Errorf("DefaultPlanKey = %s", cfg.DefaultPlanKey)
	}
	if cfg.UnitPriceUsdPerBillableMicro != "0.000001" {
		t.Errorf("UnitPriceUsdPerBillableMicro = %s", cfg.UnitPriceUsdPerBillableMicro)
	}
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pricing.json")
	os.WriteFile(path, []byte(`{
		"defaultPlanKey": "test",
		"billableFeatureKey": "bf",
		"billableMeterKey": "bm"
	}`), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.UnitPriceUsdPerBillableMicro != "0.000001" {
		t.Errorf("expected default unit price, got %s", cfg.UnitPriceUsdPerBillableMicro)
	}
	if cfg.DefaultTrialIncludedUsdMicros != "5000000" {
		t.Errorf("expected default trial included, got %s", cfg.DefaultTrialIncludedUsdMicros)
	}
}
