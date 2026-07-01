package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration for the Builder API.
type Config struct {
	Port              string
	Auth0Domain       string
	Auth0Issuer       string
	Auth0Audience     string
	MgmtClientID      string
	MgmtClientSecret  string
	SignerM2MClientID string
	SignerM2MSecret   string
	DBConnection      string
	OpenMeterURL      string
	OpenMeterAPIKey   string
	OpenMeterDefaultPlanKey string
	OpenMeterTrialFeatureKey string
	OpenMeterDefaultStarterIncludedUsdMicros int64
	SignerURL         string
	DiscoveryURL      string
	APIKeyPrefix      string
	DemoAPIKeys       string
	OIDCClientClaim   string
	OIDCSubjectClaim  string
	OIDCRequiredScopes []string
}

// Load reads configuration from environment variables.
func Load() (Config, error) {
	cfg := Config{
		Port:              envOr("BUILDER_API_PORT", "8095"),
		Auth0Domain:       firstEnv("AUTH0_DOMAIN"),
		Auth0Issuer:       firstEnv("AUTH0_ISSUER"),
		Auth0Audience:     envOr(firstEnv("AUTH0_AUDIENCE", "DEMO_APP_AUTH0_AUDIENCE"), "livepeer-clearinghouse"),
		MgmtClientID:      firstEnv("AUTH0_MGMT_CLIENT_ID"),
		MgmtClientSecret:  firstEnv("AUTH0_MGMT_CLIENT_SECRET"),
		SignerM2MClientID: firstEnv("AUTH0_SIGNER_M2M_CLIENT_ID", "DEMO_APP_AUTH0_M2M_CLIENT_ID"),
		SignerM2MSecret:   firstEnv("AUTH0_SIGNER_M2M_CLIENT_SECRET", "DEMO_APP_AUTH0_M2M_CLIENT_SECRET"),
		DBConnection:      envOr("AUTH0_DB_CONNECTION", "Username-Password-Authentication"),
		OpenMeterURL:      envOr("OPENMETER_URL", "https://us.api.konghq.com/v3/openmeter"),
		OpenMeterAPIKey:   strings.TrimSpace(os.Getenv("OPENMETER_API_KEY")),
		OpenMeterDefaultPlanKey: envOr("OPENMETER_DEFAULT_PLAN_KEY", "clearinghouse_default_ppu"),
		OpenMeterTrialFeatureKey: envOr("OPENMETER_TRIAL_FEATURE_KEY", "network_spend"),
		OpenMeterDefaultStarterIncludedUsdMicros: envInt64Or(
			"OPENMETER_DEFAULT_STARTER_INCLUDED_USD_MICROS",
			5_000_000,
		),
		SignerURL: strings.TrimSpace(os.Getenv("SIGNER_URL")),
		DiscoveryURL: envOr(
			"DISCOVERY_URL",
			"https://discovery-service-production-8955.up.railway.app/v1/discovery/raw?serviceType=legacy",
		),
		APIKeyPrefix: envOr("API_KEY_PREFIX", "sk_"),
		DemoAPIKeys:        strings.TrimSpace(os.Getenv("DEMO_API_KEYS")),
		OIDCClientClaim:    strings.TrimSpace(os.Getenv("OIDC_CLIENT_CLAIM")),
		OIDCSubjectClaim:   strings.TrimSpace(os.Getenv("OIDC_SUBJECT_CLAIM")),
		OIDCRequiredScopes: splitScopes(os.Getenv("OIDC_REQUIRED_SCOPES")),
	}

	if cfg.Auth0Issuer == "" && cfg.Auth0Domain != "" {
		cfg.Auth0Issuer = "https://" + strings.TrimSuffix(cfg.Auth0Domain, "/") + "/"
	}
	cfg.Auth0Issuer = strings.TrimSuffix(cfg.Auth0Issuer, "/")

	missing := make([]string, 0)
	if cfg.Auth0Domain == "" {
		missing = append(missing, "AUTH0_DOMAIN")
	}
	if cfg.MgmtClientID == "" {
		missing = append(missing, "AUTH0_MGMT_CLIENT_ID")
	}
	if cfg.MgmtClientSecret == "" {
		missing = append(missing, "AUTH0_MGMT_CLIENT_SECRET")
	}
	if cfg.SignerM2MClientID == "" {
		missing = append(missing, "AUTH0_SIGNER_M2M_CLIENT_ID")
	}
	if cfg.SignerM2MSecret == "" {
		missing = append(missing, "AUTH0_SIGNER_M2M_CLIENT_SECRET")
	}
	if cfg.OpenMeterAPIKey == "" {
		missing = append(missing, "OPENMETER_API_KEY")
	}
	if len(missing) > 0 {
		return cfg, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}

	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return cfg, fmt.Errorf("BUILDER_API_PORT must be numeric: %w", err)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return ""
}

func envInt64Or(key string, fallback int64) int64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func splitScopes(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
