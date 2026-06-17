package meters

import (
	"encoding/json"
	"fmt"
	"os"
)

type MeterDef struct {
	Key           string
	Name          string
	Description   string
	EventType     string
	Aggregation   string
	ValueProperty string
	Dimensions    map[string]string
}

type meterSpec struct {
	KonnectName        string `json:"konnectName"`
	KonnectDescription string `json:"konnectDescription"`
	ValueProperty      string `json:"valueProperty"`
}

type Config struct {
	CreateSignedTicketEventType string            `json:"createSignedTicketEventType"`
	SignedTicketEventSource     string            `json:"signedTicketEventSource"`
	DefaultTrialFeatureKey     string            `json:"defaultTrialFeatureKey"`
	DefaultBillableFeatureKey  string            `json:"defaultBillableFeatureKey"`
	NetworkFeeUsdMicrosMeter   string            `json:"networkFeeUsdMicrosMeter"`
	BillableUsdMicrosMeter     string            `json:"billableUsdMicrosMeter"`
	SignedTicketCountMeter     string            `json:"signedTicketCountMeter"`
	Dimensions                 map[string]string `json:"dimensions"`
	Meters                     struct {
		NetworkFeeUsdMicros meterSpec `json:"networkFeeUsdMicros"`
		BillableUsdMicros   meterSpec `json:"billableUsdMicros"`
		SignedTicketCount   meterSpec `json:"signedTicketCount"`
	} `json:"meters"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading meters config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing meters config: %w", err)
	}
	if cfg.CreateSignedTicketEventType == "" {
		return nil, fmt.Errorf("invalid meter config: createSignedTicketEventType is required")
	}
	if cfg.NetworkFeeUsdMicrosMeter == "" || cfg.SignedTicketCountMeter == "" {
		return nil, fmt.Errorf("invalid meter config: networkFeeUsdMicrosMeter and signedTicketCountMeter are required")
	}
	if cfg.BillableUsdMicrosMeter == "" {
		return nil, fmt.Errorf("invalid meter config: billableUsdMicrosMeter is required")
	}
	return &cfg, nil
}

func (c *Config) KonnectMeterDefinitions() []MeterDef {
	return []MeterDef{
		{
			Key:           c.NetworkFeeUsdMicrosMeter,
			Name:          c.Meters.NetworkFeeUsdMicros.KonnectName,
			Description:   c.Meters.NetworkFeeUsdMicros.KonnectDescription,
			EventType:     c.CreateSignedTicketEventType,
			Aggregation:   "sum",
			ValueProperty: c.Meters.NetworkFeeUsdMicros.ValueProperty,
			Dimensions:    c.Dimensions,
		},
		{
			Key:           c.BillableUsdMicrosMeter,
			Name:          c.Meters.BillableUsdMicros.KonnectName,
			Description:   c.Meters.BillableUsdMicros.KonnectDescription,
			EventType:     c.CreateSignedTicketEventType,
			Aggregation:   "sum",
			ValueProperty: c.Meters.BillableUsdMicros.ValueProperty,
			Dimensions:    c.Dimensions,
		},
		{
			Key:         c.SignedTicketCountMeter,
			Name:        c.Meters.SignedTicketCount.KonnectName,
			Description: c.Meters.SignedTicketCount.KonnectDescription,
			EventType:   c.CreateSignedTicketEventType,
			Aggregation: "count",
			Dimensions:  c.Dimensions,
		},
	}
}
