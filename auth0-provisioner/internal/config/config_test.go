package config

import "testing"

func TestParseRequiresCredentials(t *testing.T) {
	t.Setenv("AUTH0_DOMAIN", "")
	t.Setenv("AUTH0_MGMT_CLIENT_ID", "")
	t.Setenv("AUTH0_MGMT_CLIENT_SECRET", "")

	if _, err := Parse(nil); err == nil {
		t.Fatal("expected error when AUTH0_DOMAIN is missing")
	}

	t.Setenv("AUTH0_DOMAIN", "example.us.auth0.com")
	if _, err := Parse(nil); err == nil {
		t.Fatal("expected error when Management API credentials are missing")
	}
}

func TestParseDefaultsAndOverrides(t *testing.T) {
	t.Setenv("AUTH0_DOMAIN", "example.us.auth0.com")
	t.Setenv("AUTH0_MGMT_CLIENT_ID", "mgmt-id")
	t.Setenv("AUTH0_MGMT_CLIENT_SECRET", "mgmt-secret")
	t.Setenv("AUTH0_CONFIG_PATH", "")
	t.Setenv("BOOTSTRAP_OUTPUT", "")

	cfg, err := Parse(nil)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.ConfigPath != "config/auth0.yaml" {
		t.Errorf("default ConfigPath = %q, want config/auth0.yaml", cfg.ConfigPath)
	}
	if cfg.OutputPath != ".env.livepeer" {
		t.Errorf("default OutputPath = %q, want .env.livepeer", cfg.OutputPath)
	}

	cfg, err = Parse([]string{"--config", "custom.yaml", "--output", "out.env"})
	if err != nil {
		t.Fatalf("Parse with flags: %v", err)
	}
	if cfg.ConfigPath != "custom.yaml" || cfg.OutputPath != "out.env" {
		t.Errorf("flag overrides not applied: %+v", cfg)
	}
}
