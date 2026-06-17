package config

import "testing"

const testEmptyEnv = "testdata/empty.env"

func parseForTest(t *testing.T, args ...string) (*BootstrapConfig, error) {
	t.Helper()

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
	t.Setenv("AUTH0_DOMAIN", "test.auth0.com")
	t.Setenv("AUTH0_MGMT_CLIENT_ID", "cid")
	t.Setenv("AUTH0_MGMT_CLIENT_SECRET", "csec")
	t.Setenv("OPENMETER_API_KEY", "kpat_test")

	cfg, err := parseForTest(t)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Auth0Domain != "test.auth0.com" {
		t.Errorf("Auth0Domain = %s", cfg.Auth0Domain)
	}
	if cfg.AppName != "Livepeer Clearinghouse" {
		t.Errorf("AppName = %s", cfg.AppName)
	}
	if cfg.APIAudience != "livepeer-clearinghouse" {
		t.Errorf("APIAudience = %s", cfg.APIAudience)
	}
	if cfg.WebhookSecret == "" {
		t.Error("expected auto-generated webhook secret")
	}
}

func TestParseMissingAuth0(t *testing.T) {
	t.Setenv("AUTH0_DOMAIN", "")
	t.Setenv("OPENMETER_API_KEY", "kpat_test")

	_, err := parseForTest(t)
	if err == nil {
		t.Fatal("expected error for missing auth0 domain")
	}
}

func TestParseSkipAuth0(t *testing.T) {
	t.Setenv("OPENMETER_API_KEY", "kpat_test")

	cfg, err := parseForTest(t, "--skip-auth0")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.SkipAuth0 {
		t.Error("expected SkipAuth0=true")
	}
}

func TestParseSkipBoth(t *testing.T) {
	cfg, err := parseForTest(t, "--skip-auth0", "--skip-openmeter")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.SkipAuth0 || !cfg.SkipOpenMeter {
		t.Error("expected both skip flags")
	}
}
