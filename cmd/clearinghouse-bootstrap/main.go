package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/livepeer/clearinghouse/internal/admin"
	"github.com/livepeer/clearinghouse/internal/auth0"
	"github.com/livepeer/clearinghouse/internal/config"
	"github.com/livepeer/clearinghouse/internal/meters"
	"github.com/livepeer/clearinghouse/internal/output"
	"github.com/livepeer/clearinghouse/internal/pricing"
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

	ctx := context.Background()

	var auth0Result *auth0.ProvisionResult
	if !cfg.SkipAuth0 {
		log.Println("Provisioning Auth0...")
		auth0Result, err = auth0.Provision(ctx, auth0.ProvisionConfig{
			Domain:           cfg.Auth0Domain,
			MgmtClientID:     cfg.Auth0MgmtClientID,
			MgmtClientSecret: cfg.Auth0MgmtClientSecret,
			AppName:          cfg.AppName,
			APIAudience:      cfg.APIAudience,
		})
		if err != nil {
			log.Fatalf("Auth0 provisioning failed: %v", err)
		}
		log.Printf("Auth0 provisioned: API=%s PublicClient=%s M2M=%s",
			auth0Result.APIIdentifier, auth0Result.PublicClientID, auth0Result.M2MClientID)
	} else {
		log.Println("Skipping Auth0 (--skip-auth0)")
	}

	if !cfg.SkipOpenMeter {
		log.Println("Bootstrapping OpenMeter/Konnect catalog...")

		meterCfg, err := meters.Load(cfg.MetersConfigPath)
		if err != nil {
			log.Fatalf("Failed to load meters config: %v", err)
		}
		pricingCfg, err := pricing.Load(cfg.PricingConfigPath)
		if err != nil {
			log.Fatalf("Failed to load pricing config: %v", err)
		}

		omAdmin := admin.CreateAdmin(cfg.OpenmeterURL, cfg.OpenmeterAPIKey)
		result, err := admin.BootstrapCatalog(ctx, omAdmin, meterCfg, pricingCfg, cfg.TrialFeatureKey, admin.BootstrapOptions{
			Prune: cfg.Prune,
		})
		if err != nil {
			log.Fatalf("OpenMeter bootstrap failed: %v", err)
		}
		printBootstrapResult(result)
	} else {
		log.Println("Skipping OpenMeter (--skip-openmeter)")
	}

	envContent := output.BuildEnvFile(cfg, auth0Result, defaultPlanKey(cfg))
	if err := os.WriteFile(cfg.OutputPath, []byte(envContent), 0644); err != nil {
		log.Fatalf("Failed to write %s: %v", cfg.OutputPath, err)
	}
	log.Printf("Wrote %s", cfg.OutputPath)

	if auth0Result != nil {
		sdkJSON, err := output.BuildSDKConfig(cfg, auth0Result)
		if err != nil {
			log.Fatalf("Failed to build sdk-config.json: %v", err)
		}
		if err := os.WriteFile(cfg.SDKConfigOutputPath, append(sdkJSON, '\n'), 0644); err != nil {
			log.Fatalf("Failed to write %s: %v", cfg.SDKConfigOutputPath, err)
		}
		log.Printf("Wrote %s", cfg.SDKConfigOutputPath)
	}

	log.Println("Bootstrap complete.")
}

func defaultPlanKey(cfg *config.BootstrapConfig) string {
	if cfg.SkipOpenMeter {
		return ""
	}
	pricingCfg, err := pricing.Load(cfg.PricingConfigPath)
	if err != nil {
		return ""
	}
	return pricingCfg.DefaultPlanKey
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func printBootstrapResult(r *admin.BootstrapResult) {
	if r.Prune != nil {
		for _, key := range r.Prune.DeletedPlans {
			log.Printf("  pruned plan: %s", key)
		}
		for _, key := range r.Prune.DeletedFeatures {
			log.Printf("  pruned feature: %s", key)
		}
		for _, key := range r.Prune.DeletedMeters {
			log.Printf("  pruned meter: %s", key)
		}
		for _, w := range r.Prune.Warnings {
			log.Printf("  prune warning: %s", w)
		}
	}
	for _, m := range r.Meters {
		action := "exists"
		if m.Created {
			action = "created"
		}
		msg := fmt.Sprintf("  meter %s: %s", m.Resource.Key, action)
		if len(m.Warnings) > 0 {
			msg += " [" + strings.Join(m.Warnings, "; ") + "]"
		}
		log.Println(msg)
	}
	for _, f := range r.Features {
		action := "exists"
		if f.Created {
			action = "created"
		}
		msg := fmt.Sprintf("  feature %s: %s", f.Resource.Key, action)
		if len(f.Warnings) > 0 {
			msg += " [" + strings.Join(f.Warnings, "; ") + "]"
		}
		log.Println(msg)
	}
	if r.Plan != nil {
		action := "exists"
		if r.Plan.Created {
			action = "created"
		}
		log.Printf("  plan %s: %s", r.Plan.Resource.Key, action)
	}
	if r.PlanSkippedReason != "" {
		log.Printf("  plan skipped: %s", r.PlanSkippedReason)
	}

	summary := map[string]string{
		"planKey":             r.PlanKey,
		"billableFeatureKey":  r.BillableFeatureKey,
		"trialIncludedMicros": r.TrialIncludedMicros,
	}
	b, _ := json.MarshalIndent(summary, "  ", "  ")
	log.Printf("  pricing: %s", string(b))
}
