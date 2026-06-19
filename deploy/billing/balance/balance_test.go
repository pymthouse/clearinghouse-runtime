package balance_test

import (
	"testing"

	"github.com/livepeer/clearinghouse/deploy/billing/balance"
)

func TestParseEntitlementAccess(t *testing.T) {
	t.Parallel()

	raw := []byte(`{
		"data": [
			{"feature_key": "network_spend", "has_access": true},
			{"feature_key": "billable_spend", "has_access": false}
		]
	}`)

	hasAccess, err := balance.ParseEntitlementAccess(raw, "billable_spend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasAccess {
		t.Fatal("expected hasAccess=false for billable_spend")
	}

	hasAccess, err = balance.ParseEntitlementAccess(raw, "network_spend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasAccess {
		t.Fatal("expected hasAccess=true for network_spend")
	}
}

func TestParseEntitlementAccessMissingFeature(t *testing.T) {
	t.Parallel()

	raw := []byte(`{"data":[{"feature_key":"other","has_access":true}]}`)
	hasAccess, err := balance.ParseEntitlementAccess(raw, "billable_spend")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasAccess {
		t.Fatal("expected hasAccess=false when feature missing")
	}
}
