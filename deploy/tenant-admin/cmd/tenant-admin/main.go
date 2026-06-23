package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/auth0"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/billingidentity"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/config"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/httpapi"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/konnect"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/openmeter"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/registry"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/tenant"
)

func main() {
	mode := flag.String("mode", "server", "run mode: server|provision-tenant|migrate-customers|migrate-customer-keys")
	dryRun := flag.Bool("dry-run", false, "log intent without mutating systems")
	tenantID := flag.String("tenant-id", "", "tenant ID slug for provision-tenant mode")
	tenantName := flag.String("tenant-name", "", "tenant display name for provision-tenant mode")
	adminEmails := flag.String("admin-emails", "", "comma-separated admin emails for provision-tenant mode")
	adminPassword := flag.String("admin-password", "", "default admin password for provision-tenant mode")
	clientID := flag.String("client-id", "demo-client", "sample client ID for ensure-customer")
	externalUserID := flag.String("external-user-id", "demo-user", "sample external user ID for ensure-customer")
	enableSampleUser := flag.Bool("enable-sample-user", true, "provision a sample customer after tenant creation")
	flag.Parse()

	cfg, err := config.LoadFromEnv()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	konnectSDK := konnect.NewSDK(cfg.KonnectAPIURL, cfg.KonnectPlatformToken)
	konnectManager := &konnect.Manager{
		SDK: konnectSDK,
	}

	auth0Client := auth0.NewClient(cfg.Auth0Domain, cfg.Auth0MgmtClientID, cfg.Auth0MgmtClientSecret)
	fileRegistry := registry.NewFileRegistry(cfg.DataDir)

	provisioner := &tenant.Provisioner{
		Auth0:   auth0Client,
		Konnect: konnectManager,
		OpenMeterFactory: func(token string) tenant.OpenMeterProvisioner {
			return openmeter.NewProvisioner(cfg.OpenMeterURL, token, cfg.OpenMeterDefaultPlanKey)
		},
		OpenMeterURL:    cfg.OpenMeterURL,
		Registry:        fileRegistry,
		DataDir:         cfg.DataDir,
		IngestRole:      cfg.KonnectIngestRole,
		SpatTTL:         cfg.SpatTTL,
		Auth0Connection: cfg.Auth0DefaultConnection,
		DryRun:          *dryRun,
	}

	switch strings.TrimSpace(*mode) {
	case "server":
		server := httpapi.NewServer(cfg.AdminSecret, provisioner)
		log.Printf("tenant-admin listening on %s (dry_run=%v)", cfg.ListenAddr, *dryRun)
		if err := http.ListenAndServe(cfg.ListenAddr, server.Handler()); err != nil {
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
		fmt.Fprintf(os.Stdout, "tenant=%s auth0_org=%s konnect_team=%s system_account=%s env=%s\n",
			result.TenantID,
			result.Auth0Organization,
			result.KonnectTeamID,
			result.SystemAccountID,
			result.EnvFile,
		)
	case "migrate-customers", "migrate-customer-keys":
		derivedCustomerKey, err := billingidentity.BuildCustomerKey(
			strings.TrimSpace(*tenantID),
			strings.TrimSpace(*clientID),
			strings.TrimSpace(*externalUserID),
		)
		if err != nil {
			log.Fatalf("migrate-customers invalid identity: %v", err)
		}
		if *dryRun {
			fmt.Fprintf(os.Stdout, "dry_run tenant=%s legacy=%s:%s surrogate=%s\n",
				strings.TrimSpace(*tenantID),
				strings.TrimSpace(*clientID),
				strings.TrimSpace(*externalUserID),
				derivedCustomerKey,
			)
			return
		}
		result, ensureErr := provisioner.EnsureCustomer(
			context.Background(),
			strings.TrimSpace(*tenantID),
			strings.TrimSpace(*clientID),
			strings.TrimSpace(*externalUserID),
		)
		if ensureErr != nil {
			log.Fatalf("migrate-customers failed: %v", ensureErr)
		}
		fmt.Fprintf(os.Stdout, "tenant=%s customer_key=%s status=%s\n",
			strings.TrimSpace(*tenantID),
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
