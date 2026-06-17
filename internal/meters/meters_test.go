package meters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meters.json")
	os.WriteFile(path, []byte(`{
		"createSignedTicketEventType": "create_signed_ticket",
		"signedTicketEventSource": "go-livepeer-remote-signer",
		"defaultTrialFeatureKey": "network_spend",
		"defaultBillableFeatureKey": "billable_spend",
		"networkFeeUsdMicrosMeter": "network_fee_usd_micros",
		"billableUsdMicrosMeter": "billable_usd_micros",
		"signedTicketCountMeter": "signed_ticket_count",
		"dimensions": {"client_id": "$.client_id"},
		"meters": {
			"networkFeeUsdMicros": {"konnectName": "NF", "konnectDescription": "desc", "valueProperty": "$.nf"},
			"billableUsdMicros": {"konnectName": "BU", "konnectDescription": "desc", "valueProperty": "$.bu"},
			"signedTicketCount": {"konnectName": "SC", "konnectDescription": "desc"}
		}
	}`), 0644)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	defs := cfg.KonnectMeterDefinitions()
	if len(defs) != 3 {
		t.Fatalf("expected 3 meter definitions, got %d", len(defs))
	}
	if defs[0].Key != "network_fee_usd_micros" {
		t.Errorf("first meter key = %s, want network_fee_usd_micros", defs[0].Key)
	}
	if defs[0].Aggregation != "sum" {
		t.Errorf("first meter aggregation = %s, want sum", defs[0].Aggregation)
	}
	if defs[2].Aggregation != "count" {
		t.Errorf("third meter aggregation = %s, want count", defs[2].Aggregation)
	}
}

func TestLoadMissingFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "meters.json")
	os.WriteFile(path, []byte(`{"createSignedTicketEventType": ""}`), 0644)

	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing fields")
	}
}
