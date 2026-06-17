package output

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/livepeer/clearinghouse/internal/auth0"
	"github.com/livepeer/clearinghouse/internal/config"
)

func TestBuildSDKConfig(t *testing.T) {
	cfg := &config.BootstrapConfig{
		Auth0Domain:     "test.us.auth0.com",
		OpenmeterURL:    "https://us.api.konghq.com/v3/openmeter",
		TrialFeatureKey:        "network_spend",
		RemoteSignerWebhookURL: config.DefaultDockerWebhookURL,
	}
	auth0Result := &auth0.ProvisionResult{
		APIIdentifier:   "livepeer",
		PublicClientID:  "pub_123",
		M2MClientID:     "m2m_456",
		M2MClientSecret: "secret_789",
		JwksURL:         "https://test.us.auth0.com/.well-known/jwks.json",
		Issuer:          "https://test.us.auth0.com/",
	}

	got, err := BuildSDKConfig(cfg, auth0Result)
	if err != nil {
		t.Fatalf("BuildSDKConfig: %v", err)
	}

	golden := filepath.Join("testdata", "sdkconfig.golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		os.MkdirAll("testdata", 0755)
		os.WriteFile(golden, append(got, '\n'), 0644)
		t.Log("Updated golden file")
		return
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("Missing golden file %s — run with UPDATE_GOLDEN=1 to create", golden)
	}

	var gotObj, wantObj SDKConfig
	json.Unmarshal(got, &gotObj)
	json.Unmarshal(want, &wantObj)

	gotNorm, _ := json.Marshal(gotObj)
	wantNorm, _ := json.Marshal(wantObj)

	if string(gotNorm) != string(wantNorm) {
		t.Errorf("sdk-config mismatch.\n--- got ---\n%s\n--- want ---\n%s", string(got), string(want))
	}
}
