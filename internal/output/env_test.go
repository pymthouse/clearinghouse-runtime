package output

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/livepeer/clearinghouse/internal/auth0"
	"github.com/livepeer/clearinghouse/internal/config"
)

func TestBuildEnvFile(t *testing.T) {
	cfg := &config.BootstrapConfig{
		Auth0Domain:     "test.us.auth0.com",
		OpenmeterURL:    "https://us.api.konghq.com/v3/openmeter",
		OpenmeterAPIKey: "kpat_test123",
		TrialFeatureKey: "network_spend",
		WebhookSecret:   "deadbeef",
	}
	auth0Result := &auth0.ProvisionResult{
		APIIdentifier:   "livepeer",
		PublicClientID:  "pub_123",
		M2MClientID:     "m2m_456",
		M2MClientSecret: "secret_789",
		JwksURL:         "https://test.us.auth0.com/.well-known/jwks.json",
		Issuer:          "https://test.us.auth0.com/",
	}

	got := BuildEnvFile(cfg, auth0Result)

	golden := filepath.Join("testdata", "env.golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		os.MkdirAll("testdata", 0755)
		os.WriteFile(golden, []byte(got), 0644)
		t.Log("Updated golden file")
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("Missing golden file %s — run with UPDATE_GOLDEN=1 to create", golden)
	}
	if got != string(want) {
		t.Errorf("env output mismatch.\n--- got ---\n%s\n--- want ---\n%s", got, string(want))
	}
}

func TestBuildEnvFileSkipAuth0(t *testing.T) {
	cfg := &config.BootstrapConfig{
		SkipAuth0:       true,
		OpenmeterURL:    "https://us.api.konghq.com/v3/openmeter",
		OpenmeterAPIKey: "kpat_test",
		TrialFeatureKey: "network_spend",
		WebhookSecret:   "abc",
	}

	got := BuildEnvFile(cfg, nil)

	if strings.Contains(got, "AUTH0_DOMAIN") {
		t.Error("env should not contain Auth0 block when skipped")
	}
	if !strings.Contains(got, "OPENMETER_URL=") {
		t.Error("env should contain OPENMETER_URL")
	}
}
