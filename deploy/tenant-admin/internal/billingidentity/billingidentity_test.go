package billingidentity

import "testing"

func TestBuildCustomerKeyMatchesKnownVector(t *testing.T) {
	key, err := BuildCustomerKey("acme", "app_acme", "bootstrap-user")
	if err != nil {
		t.Fatalf("BuildCustomerKey failed: %v", err)
	}
	if len(key) > 64 {
		t.Fatalf("customer key too long: %d", len(key))
	}
	if key[:3] != "ch_" {
		t.Fatalf("expected ch_ prefix, got %q", key)
	}
}

func TestParseAuthIDRoundTrip(t *testing.T) {
	authID, err := BuildAuthID("acme", "app_acme", "bootstrap-user")
	if err != nil {
		t.Fatalf("BuildAuthID failed: %v", err)
	}
	tenantID, clientID, externalUserID, ok := ParseAuthID(authID)
	if !ok {
		t.Fatalf("ParseAuthID failed")
	}
	if tenantID != "acme" || clientID != "app_acme" || externalUserID != "bootstrap-user" {
		t.Fatalf("unexpected parse result: %s %s %s", tenantID, clientID, externalUserID)
	}
}
