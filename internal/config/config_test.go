package config

import "testing"

const testEmptyEnv = "testdata/empty.env"

func parseForTest(args ...string) (*BootstrapConfig, error) {
	full := append([]string{"--env-file", testEmptyEnv}, args...)
	remaining, envFile, explicit, help, err := PreprocessArgs(full)
	if err != nil {
		return nil, err
	}
	if help != HelpNone {
		return nil, nil
	}
	if err := LoadEnvFile(envFile, explicit); err != nil {
		return nil, err
	}
	return Parse(remaining)
}

func TestParseMinimal(t *testing.T) {
	cfg, err := parseForTest(
		"--auth0-domain", "test.auth0.com",
		"--auth0-mgmt-client-id", "cid",
		"--auth0-mgmt-client-secret", "csec",
		"--openmeter-api-key", "kpat_test",
	)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Auth0Domain != "test.auth0.com" {
		t.Errorf("Auth0Domain = %s", cfg.Auth0Domain)
	}
	if cfg.AppName != "Livepeer Platform" {
		t.Errorf("AppName = %s", cfg.AppName)
	}
	if cfg.APIAudience != "livepeer" {
		t.Errorf("APIAudience = %s", cfg.APIAudience)
	}
	if cfg.WebhookSecret == "" {
		t.Error("expected auto-generated webhook secret")
	}
}

func TestParseMissingAuth0(t *testing.T) {
	_, err := parseForTest("--openmeter-api-key", "kpat_test")
	if err == nil {
		t.Fatal("expected error for missing auth0 domain")
	}
}

func TestParseSkipAuth0(t *testing.T) {
	cfg, err := parseForTest("--skip-auth0", "--openmeter-api-key", "kpat_test")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.SkipAuth0 {
		t.Error("expected SkipAuth0=true")
	}
}

func TestParseSkipBoth(t *testing.T) {
	cfg, err := parseForTest("--skip-auth0", "--skip-openmeter")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.SkipAuth0 || !cfg.SkipOpenMeter {
		t.Error("expected both skip flags")
	}
}
