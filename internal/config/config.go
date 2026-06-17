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

	// Identity webhook
	WebhookSecret string

	// Signer
	SignerProxyURL         string
	SignerPublicURL        string
	RemoteSignerWebhookURL string
	SignerNetwork          string
	EthRPCURL              string
	SignerEthAddr          string

	// Kafka
	KafkaBrokers      string
	KafkaGatewayTopic string

	// Collector
	EthUsdPrice string

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

func Parse(args []string) (*BootstrapConfig, error) {
	fs := flag.NewFlagSet("clearinghouse-bootstrap", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	cfg := &BootstrapConfig{}

	fs.StringVar(&cfg.Auth0Domain, "auth0-domain", envOr("AUTH0_DOMAIN", ""), "Auth0 tenant domain")
	fs.StringVar(&cfg.Auth0MgmtClientID, "auth0-mgmt-client-id", envOr("AUTH0_MGMT_CLIENT_ID", ""), "Auth0 Management M2M client ID")
	fs.StringVar(&cfg.Auth0MgmtClientSecret, "auth0-mgmt-client-secret", envOr("AUTH0_MGMT_CLIENT_SECRET", ""), "Auth0 Management M2M client secret")
	fs.StringVar(&cfg.AppName, "app-name", envOr("APP_NAME", "Livepeer Platform"), "Application display name prefix")
	fs.StringVar(&cfg.APIAudience, "api-audience", envOr("AUTH0_AUDIENCE", "livepeer"), "API identifier / audience")
	fs.StringVar(&cfg.OpenmeterURL, "openmeter-url", envOr("OPENMETER_URL", "https://us.api.konghq.com/v3/openmeter"), "Konnect metering base URL")
	fs.StringVar(&cfg.OpenmeterAPIKey, "openmeter-api-key", envOr("OPENMETER_API_KEY", ""), "Konnect PAT (kpat_…)")
	fs.StringVar(&cfg.TrialFeatureKey, "trial-feature-key", envOr("OPENMETER_TRIAL_FEATURE_KEY", "network_spend"), "Entitlement feature key for trial")
	fs.StringVar(&cfg.WebhookSecret, "webhook-secret", envOr("WEBHOOK_SECRET", ""), "Shared secret for remote_signer_webhook")
	fs.StringVar(&cfg.SignerProxyURL, "signer-proxy-url", envOr("SIGNER_PROXY_URL", "https://your-platform.vercel.app/api/signer"), "Vercel signer proxy BFF URL")
	fs.StringVar(&cfg.SignerPublicURL, "signer-public-url", envOr("SIGNER_PUBLIC_URL", "https://signer.your-domain.com"), "Public HTTPS URL of VM remote signer")
	fs.StringVar(&cfg.RemoteSignerWebhookURL, "remote-signer-webhook-url", envOr("REMOTE_SIGNER_WEBHOOK_URL", "https://your-platform.vercel.app/webhooks/remote-signer"), "Identity webhook (go-livepeer calls this)")
	fs.StringVar(&cfg.SignerNetwork, "signer-network", envOr("SIGNER_NETWORK", "arbitrum-one-mainnet"), "go-livepeer -network")
	fs.StringVar(&cfg.EthRPCURL, "eth-rpc-url", envOr("ETH_RPC_URL", "https://arb1.arbitrum.io/rpc"), "Ethereum RPC for remote signer")
	fs.StringVar(&cfg.SignerEthAddr, "signer-eth-addr", envOr("SIGNER_ETH_ADDR", ""), "Funded signer ETH address")
	fs.StringVar(&cfg.KafkaBrokers, "kafka-brokers", envOr("KAFKA_BROKERS", "kafka:9092"), "Kafka bootstrap servers")
	fs.StringVar(&cfg.KafkaGatewayTopic, "kafka-gateway-topic", envOr("KAFKA_GATEWAY_TOPIC", "livepeer-gateway-events"), "go-livepeer monitor topic")
	fs.StringVar(&cfg.EthUsdPrice, "eth-usd-price", envOr("ETH_USD_PRICE", "3500"), "ETH/USD for collector Wei conversion")
	fs.StringVar(&cfg.OutputPath, "output", envOr("BOOTSTRAP_OUTPUT", ".env.livepeer"), "Write combined .env file")
	fs.StringVar(&cfg.SDKConfigOutputPath, "sdk-config-output", envOr("SDK_CONFIG_OUTPUT", "sdk-config.json"), "Write builder-sdk config JSON")
	fs.StringVar(&cfg.MetersConfigPath, "meters-config", envOr("METERS_CONFIG_PATH", "config/meters.json"), "Meter definition config JSON")
	fs.StringVar(&cfg.PricingConfigPath, "pricing-config", envOr("PRICING_CONFIG_PATH", "config/pricing.json"), "Pricing definition config JSON")
	fs.BoolVar(&cfg.SkipAuth0, "skip-auth0", false, "Only bootstrap OpenMeter")
	fs.BoolVar(&cfg.SkipOpenMeter, "skip-openmeter", false, "Only provision Auth0")
	fs.BoolVar(&cfg.Prune, "prune", false, "Destructive: remove Konnect catalog objects not in config")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.WebhookSecret == "" {
		cfg.WebhookSecret = randomHex(24)
	}

	if !cfg.SkipAuth0 {
		if cfg.Auth0Domain == "" {
			return nil, fmt.Errorf("missing AUTH0_DOMAIN (set in .env or pass --auth0-domain)")
		}
		if cfg.Auth0MgmtClientID == "" || cfg.Auth0MgmtClientSecret == "" {
			return nil, fmt.Errorf("missing Auth0 Management API credentials (AUTH0_MGMT_CLIENT_ID / AUTH0_MGMT_CLIENT_SECRET in .env)")
		}
	}

	if !cfg.SkipOpenMeter {
		if cfg.OpenmeterURL == "" {
			return nil, fmt.Errorf("missing OPENMETER_URL (set in .env or pass --openmeter-url)")
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
Settings load from .env (see .env.example); flags override env.

Quick start:
  cp .env.example .env
  ./clearinghouse-bootstrap

Options:
  --env-file <path>             Env file (default: .env if present)
  --skip-auth0                  Konnect catalog only
  --skip-openmeter              Auth0 only
  --prune                       Remove catalog objects not in config (destructive)
  --output <path>               .env output (default: .env.livepeer)
  --sdk-config-output <path>    JSON output (default: sdk-config.json)

Run --help-all for config file locations.
`)
}

func printHelpAll() {
	fmt.Print(`Configuration reference

  .env.example              Auth0, Konnect, deploy vars, output paths
  config/meters.json        Meter definitions
  config/pricing.json       Default plan and pricing
`)
}
