package pricing

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	DefaultTrialIncludedUsdMicros string `json:"defaultTrialIncludedUsdMicros"`
	DefaultPlanKey                string `json:"defaultPlanKey"`
	BillableFeatureKey            string `json:"billableFeatureKey"`
	BillableMeterKey              string `json:"billableMeterKey"`
	UnitPriceUsdPerBillableMicro  string `json:"unitPriceUsdPerBillableMicro"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading pricing config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing pricing config: %w", err)
	}
	if cfg.DefaultPlanKey == "" {
		return nil, fmt.Errorf("invalid pricing config: defaultPlanKey is required")
	}
	if cfg.BillableFeatureKey == "" || cfg.BillableMeterKey == "" {
		return nil, fmt.Errorf("invalid pricing config: billableFeatureKey and billableMeterKey are required")
	}
	if cfg.UnitPriceUsdPerBillableMicro == "" {
		cfg.UnitPriceUsdPerBillableMicro = "0.000001"
	}
	if cfg.DefaultTrialIncludedUsdMicros == "" {
		cfg.DefaultTrialIncludedUsdMicros = "5000000"
	}
	return &cfg, nil
}
