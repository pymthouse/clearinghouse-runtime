package config

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"strings"
)

type BootstrapConfig struct {
	// Auth0
	Auth0Domain           string
	Auth0MgmtClientID     string
	Auth0MgmtClientSecret string
	AppName               string
	APIAudience           string

	// OpenMeter / Konnect
	OpenmeterURL    string
	OpenmeterAPIKey string
	TrialFeatureKey string

	// Identity webhook (auto-generated when unset)
	WebhookSecret            string
	PlatformURL              string
	RemoteSignerWebhookURL   string

	// Output
	OutputPath          string
	SDKConfigOutputPath string

	// Config files
	MetersConfigPath  string
	PricingConfigPath string

	// Skip flags
	SkipAuth0     bool
	SkipOpenMeter bool

	// Destructive catalog sync
	Prune bool
}

func env(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func envOr(key, fallback string) string {
	if v := env(key); v != "" {
		return v
	}
	return fallback
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func loadFromEnv() *BootstrapConfig {
	return &BootstrapConfig{
		Auth0Domain:           envOr("AUTH0_DOMAIN", ""),
		Auth0MgmtClientID:     envOr("AUTH0_MGMT_CLIENT_ID", ""),
		Auth0MgmtClientSecret: envOr("AUTH0_MGMT_CLIENT_SECRET", ""),
		AppName:               envOr("APP_NAME", "Livepeer Clearinghouse"),
		APIAudience:           envOr("AUTH0_AUDIENCE", "livepeer-clearinghouse"),
		OpenmeterURL:          envOr("OPENMETER_URL", "https://us.api.konghq.com/v3/openmeter"),
		OpenmeterAPIKey:       envOr("OPENMETER_API_KEY", ""),
		TrialFeatureKey:       envOr("OPENMETER_TRIAL_FEATURE_KEY", "network_spend"),
		WebhookSecret:          envOr("WEBHOOK_SECRET", ""),
		PlatformURL:            envOr("PLATFORM_URL", ""),
		RemoteSignerWebhookURL: envOr("REMOTE_SIGNER_WEBHOOK_URL", ""),
		OutputPath:             envOr("BOOTSTRAP_OUTPUT", ".env.livepeer"),
		SDKConfigOutputPath:   envOr("SDK_CONFIG_OUTPUT", "sdk-config.json"),
		MetersConfigPath:      envOr("METERS_CONFIG_PATH", "config/meters.json"),
		PricingConfigPath:     envOr("PRICING_CONFIG_PATH", "config/pricing.json"),
	}
}

func Parse(args []string) (*BootstrapConfig, error) {
	fs := flag.NewFlagSet("clearinghouse-bootstrap", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	cfg := loadFromEnv()

	fs.BoolVar(&cfg.SkipAuth0, "skip-auth0", false, "Only bootstrap OpenMeter")
	fs.BoolVar(&cfg.SkipOpenMeter, "skip-openmeter", false, "Only provision Auth0")
	fs.BoolVar(&cfg.Prune, "prune", false, "Destructive: remove Konnect catalog objects not in config")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.WebhookSecret == "" {
		cfg.WebhookSecret = randomHex(24)
	}

	cfg.RemoteSignerWebhookURL = ResolveRemoteSignerWebhookURL(
		cfg.PlatformURL,
		cfg.RemoteSignerWebhookURL,
	)

	if !cfg.SkipAuth0 {
		if cfg.Auth0Domain == "" {
			return nil, fmt.Errorf("missing AUTH0_DOMAIN in .env")
		}
		if cfg.Auth0MgmtClientID == "" || cfg.Auth0MgmtClientSecret == "" {
			return nil, fmt.Errorf("missing Auth0 Management API credentials (AUTH0_MGMT_CLIENT_ID / AUTH0_MGMT_CLIENT_SECRET in .env)")
		}
	}

	if !cfg.SkipOpenMeter {
		if cfg.OpenmeterURL == "" {
			return nil, fmt.Errorf("missing OPENMETER_URL in .env")
		}
		if cfg.OpenmeterAPIKey == "" {
			return nil, fmt.Errorf("missing OPENMETER_API_KEY — Konnect PAT (kpat_…) in .env")
		}
	}

	return cfg, nil
}

// PrintUsage prints brief or full help.
func PrintUsage(all bool) {
	if all {
		printHelpAll()
		return
	}
	printHelpBrief()
}

func printHelpBrief() {
	fmt.Print(`Usage: clearinghouse-bootstrap [options]

Provision Auth0 + OpenMeter/Konnect and write .env.livepeer + sdk-config.json.
Settings load from .env (see .env.example).

Quick start:
  cp .env.example .env
  ./clearinghouse-bootstrap

Options:
  --env-file <path>    Env file (default: .env if present)
  --skip-auth0         Konnect catalog only
  --skip-openmeter     Auth0 only
  --prune              Remove catalog objects not in config (destructive)

Run --help-all for config file locations.
`)
}

func printHelpAll() {
	fmt.Print(`Configuration reference

  .env.example              Auth0 and Konnect credentials, output paths
  config/meters.json        Meter definitions
  config/pricing.json       Default plan and pricing
`)
}
