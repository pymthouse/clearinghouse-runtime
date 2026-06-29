package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/livepeer/clearinghouse/tenant-admin/internal/auth0"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/config"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/httpapi"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/konnect"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/openmeter"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/registry"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/tenant"
)

func main() {
	mode := flag.String("mode", "server", "run mode: server|provision-tenant|ensure-customer")
	dryRun := flag.Bool("dry-run", false, "log intent without mutating systems")
	tenantID := flag.String("tenant-id", "", "tenant ID slug")
	tenantName := flag.String("tenant-name", "", "tenant display name (provision-tenant)")
	adminEmails := flag.String("admin-emails", "", "comma-separated admin emails (provision-tenant)")
	adminPassword := flag.String("admin-password", "", "default admin password (provision-tenant)")
	clientID := flag.String("client-id", "demo-client", "sample/target client ID")
	externalUserID := flag.String("external-user-id", "demo-user", "sample/target external user ID")
	enableSampleUser := flag.Bool("enable-sample-user", true, "provision a sample customer after tenant creation")
	flag.Parse()

	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	konnectManager := &konnect.Manager{
		SDK: konnect.NewSDK(cfg.KonnectAPIURL, cfg.KonnectPlatformToken),
	}
	auth0Client := auth0.NewClient(cfg.Auth0Domain, cfg.Auth0MgmtClientID, cfg.Auth0MgmtClientSecret)
	fileRegistry := registry.NewFileRegistry(cfg.DataDir)
	openMeterProvisioner := openmeter.NewProvisioner(cfg.OpenMeterURL, cfg.OpenMeterProvisionerPAT, cfg.OpenMeterDefaultPlanKey)

	provisioner := &tenant.Provisioner{
		Auth0:           auth0Client,
		Konnect:         konnectManager,
		OpenMeter:       openMeterProvisioner,
		Registry:        fileRegistry,
		IngestRole:      cfg.KonnectIngestRole,
		SpatTTL:         cfg.SpatTTL,
		Auth0Connection: cfg.Auth0DefaultConnection,
		DryRun:          *dryRun,
	}

	switch strings.TrimSpace(*mode) {
	case "server":
		server := httpapi.NewServer(cfg.AdminSecret, cfg.InternalAPISecret, provisioner)
		go func() {
			log.Printf("tenant-admin internal API listening on %s (loopback only)", cfg.InternalListenAddr)
			if err := http.ListenAndServe(cfg.InternalListenAddr, server.InternalHandler()); err != nil {
				log.Fatalf("internal listen error: %v", err)
			}
		}()
		log.Printf("tenant-admin admin API listening on %s (dry_run=%v)", cfg.ListenAddr, *dryRun)
		if err := http.ListenAndServe(cfg.ListenAddr, server.AdminHandler()); err != nil {
			log.Fatalf("listen error: %v", err)
		}
	case "provision-tenant":
		result, err := provisioner.ProvisionTenant(context.Background(), tenant.TenantProvisionInput{
			TenantID:         strings.TrimSpace(*tenantID),
			TenantName:       strings.TrimSpace(*tenantName),
			AdminEmails:      splitCSV(*adminEmails),
			AdminPassword:    strings.TrimSpace(*adminPassword),
			ClientID:         strings.TrimSpace(*clientID),
			ExternalUserID:   strings.TrimSpace(*externalUserID),
			EnableSampleUser: *enableSampleUser,
		})
		if err != nil {
			log.Fatalf("provision-tenant failed: %v", err)
		}
		fmt.Fprintf(os.Stdout, "tenant=%s auth0_org=%s konnect_team=%s ingest_account=%s ingest_token=%s\n",
			result.TenantID,
			result.Auth0Organization,
			result.KonnectTeamID,
			result.IngestAccountID,
			result.IngestTokenID,
		)
	case "ensure-customer":
		result, err := provisioner.EnsureCustomer(
			context.Background(),
			strings.TrimSpace(*clientID),
			strings.TrimSpace(*externalUserID),
		)
		if err != nil {
			log.Fatalf("ensure-customer failed: %v", err)
		}
		fmt.Fprintf(os.Stdout, "customer_key=%s status=%s\n",
			result.CustomerKey,
			result.Status,
		)
	default:
		log.Fatalf("unsupported mode %q", *mode)
	}
}

func splitCSV(raw string) []string {
	values := strings.Split(raw, ",")
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
