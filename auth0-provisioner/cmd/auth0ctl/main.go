// Command auth0ctl idempotently provisions an Auth0 tenant slice — tenant
// device-flow settings, resource server(s), and public/confidential client pairs
// modeling pymthouse's OIDC issuer design — from a declarative config/auth0.yaml.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/livepeer/clearinghouse/auth0-provisioner/internal/auth0"
	"github.com/livepeer/clearinghouse/auth0-provisioner/internal/config"
	"github.com/livepeer/clearinghouse/auth0-provisioner/internal/output"
)

func main() {
	args, envFile, envFileExplicit, help, err := config.PreprocessArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		config.PrintUsage(false)
		os.Exit(1)
	}
	if help != config.HelpNone {
		config.PrintUsage(help == config.HelpAll)
		os.Exit(0)
	}

	if err := config.LoadEnvFile(envFile, envFileExplicit); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	if envFileExplicit || fileExists(envFile) {
		log.Printf("Loaded config from %s", envFile)
	}

	cfg, err := config.Parse(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n\n", err)
		config.PrintUsage(false)
		os.Exit(1)
	}

	cat, err := config.LoadCatalog(cfg.ConfigPath)
	if err != nil {
		log.Fatalf("Failed to load Auth0 config: %v", err)
	}
	log.Printf("Loaded %s: %d resource server(s), %d app pair(s)",
		cfg.ConfigPath, len(cat.ResourceServers), len(cat.Apps))

	ctx := context.Background()
	log.Println("Provisioning Auth0...")
	result, err := auth0.Provision(ctx, auth0.Runtime{
		Domain:           cfg.Auth0Domain,
		MgmtClientID:     cfg.Auth0MgmtClientID,
		MgmtClientSecret: cfg.Auth0MgmtClientSecret,
	}, cat, func(format string, a ...any) { log.Printf("  "+format, a...) })
	if err != nil {
		log.Fatalf("Auth0 provisioning failed: %v", err)
	}

	envContent := output.BuildEnvFile(result)
	if err := os.WriteFile(cfg.OutputPath, []byte(envContent), 0o600); err != nil {
		log.Fatalf("Failed to write %s: %v", cfg.OutputPath, err)
	}
	log.Printf("Wrote %s (%d app pair(s))", cfg.OutputPath, len(result.Apps))
	log.Println("Auth0 provisioning complete.")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
