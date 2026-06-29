package config

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

// Config holds the runtime settings for auth0ctl: the Auth0 Management API
// connection, the declarative config path, and the output path. Credentials and
// defaults come from the environment (loaded from .env); flags override paths.
type Config struct {
	// Auth0 Management API (the bootstrap M2M app).
	Auth0Domain           string
	Auth0MgmtClientID     string
	Auth0MgmtClientSecret string

	// Declarative tenant slice (the catalog.json analog).
	ConfigPath string

	// Generated env output (client ids/secrets). Contains secrets — never commit.
	OutputPath string
}

func env(key string) string { return strings.TrimSpace(os.Getenv(key)) }

func envOr(key, fallback string) string {
	if v := env(key); v != "" {
		return v
	}
	return fallback
}

func loadFromEnv() *Config {
	return &Config{
		Auth0Domain:           env("AUTH0_DOMAIN"),
		Auth0MgmtClientID:     env("AUTH0_MGMT_CLIENT_ID"),
		Auth0MgmtClientSecret: env("AUTH0_MGMT_CLIENT_SECRET"),
		ConfigPath:            envOr("AUTH0_CONFIG_PATH", "config/auth0.yaml"),
		OutputPath:            envOr("BOOTSTRAP_OUTPUT", ".env.livepeer"),
	}
}

// Parse builds a Config from the environment, then applies flag overrides, and
// validates that the Management API credentials are present.
func Parse(args []string) (*Config, error) {
	fs := flag.NewFlagSet("auth0ctl", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	cfg := loadFromEnv()

	fs.StringVar(&cfg.ConfigPath, "config", cfg.ConfigPath, "Path to the declarative Auth0 config (auth0.yaml)")
	fs.StringVar(&cfg.OutputPath, "output", cfg.OutputPath, "Path to write the generated env file")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if cfg.Auth0Domain == "" {
		return nil, fmt.Errorf("missing AUTH0_DOMAIN in .env")
	}
	if cfg.Auth0MgmtClientID == "" || cfg.Auth0MgmtClientSecret == "" {
		return nil, fmt.Errorf("missing Auth0 Management API credentials (AUTH0_MGMT_CLIENT_ID / AUTH0_MGMT_CLIENT_SECRET in .env)")
	}

	return cfg, nil
}

// PrintUsage prints brief or full help.
func PrintUsage(all bool) {
	if all {
		fmt.Print(`Configuration reference

  .env / .env.example       AUTH0_DOMAIN, AUTH0_MGMT_CLIENT_ID, AUTH0_MGMT_CLIENT_SECRET
  config/auth0.yaml         Declarative tenant, resource servers, and public/M2M client pairs
  .env.livepeer (output)    Generated client ids/secrets — do not commit
`)
		return
	}
	fmt.Print(`Usage: auth0ctl [options]

Idempotently provision an Auth0 tenant slice — resource server(s) and public/M2M
client pairs with RFC 8628 device flow — from config/auth0.yaml. Re-running is safe:
existing objects are updated in place, never duplicated.

Settings load from .env (see .env.example).

Quick start:
  cp .env.example .env        # fill in AUTH0_DOMAIN + Management API creds
  ./auth0ctl

Options:
  --config <path>      Declarative Auth0 config (default: config/auth0.yaml)
  --output <path>      Generated env output (default: .env.livepeer)
  --env-file <path>    Env file (default: .env if present)
  --help, --help-all   Show help

Run --help-all for config file locations.
`)
}
