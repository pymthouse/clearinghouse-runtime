package tenant

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/auth0"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/billingidentity"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/konnect"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/openmeter"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/registry"
)

type TenantProvisionInput struct {
	TenantID         string
	TenantName       string
	AdminEmails      []string
	AdminPassword    string
	ClientID         string
	ExternalUserID   string
	EnableSampleUser bool
}

type TenantProvisionResult struct {
	TenantID          string   `json:"tenantId"`
	Auth0Organization string   `json:"auth0OrgId"`
	KonnectTeamID     string   `json:"konnectTeamId"`
	SystemAccountID   string   `json:"systemAccountId"`
	SystemAccountSPAT string   `json:"spat"`
	AdminUserIDs      []string `json:"adminUsers"`
	EnvFile           string   `json:"envFile"`
}

type Auth0Provisioner interface {
	EnsureTenant(ctx context.Context, input auth0.EnsureTenantInput) (*auth0.EnsureTenantResult, error)
}

type KonnectProvisioner interface {
	EnsureTeamByName(ctx context.Context, teamName string, description string, labels map[string]string) (string, bool, error)
	EnsureSystemAccountByName(ctx context.Context, name string, description string) (string, bool, error)
	CreateSystemAccountToken(ctx context.Context, accountID string, tokenName string, expiresAt time.Time) (*konnect.SystemAccountToken, error)
	DeleteSystemAccountToken(ctx context.Context, accountID string, tokenID string) error
	EnsureSystemAccountInTeam(ctx context.Context, teamID string, accountID string) error
	EnsureRoleAssignedToSystemAccount(ctx context.Context, accountID string, roleName string) error
}

type OpenMeterProvisioner interface {
	EnsureDefaultCatalog(ctx context.Context) error
	Ensure(ctx context.Context, input openmeter.ProvisionInput) (*openmeter.ProvisionResult, error)
}

type Registry interface {
	Upsert(record registry.TenantRecord) error
	UpsertApp(record registry.TenantAppRecord) error
	Get(tenantID string) (*registry.TenantRecord, error)
	List() ([]registry.TenantRecord, error)
	ListApps(tenantID string) ([]registry.TenantAppRecord, error)
	GetSPAT(tenantID string) (string, error)
	GetTenantForClient(clientID string) (*registry.TenantRecord, error)
}

type Provisioner struct {
	Auth0            Auth0Provisioner
	Konnect          KonnectProvisioner
	OpenMeterFactory func(token string) OpenMeterProvisioner
	OpenMeterURL     string
	Registry         Registry
	DataDir          string
	IngestRole       string
	SpatTTL          time.Duration
	Auth0Connection  string
	DryRun           bool
}

var requiredMeteringRoles = []string{
	"Meter Admin",
	"Product Catalog Admin",
	"Billing Admin",
}

func (p *Provisioner) ProvisionTenant(ctx context.Context, input TenantProvisionInput) (*TenantProvisionResult, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	if p.DryRun {
		return &TenantProvisionResult{
			TenantID: input.TenantID,
		}, nil
	}

	auth0Result, err := p.Auth0.EnsureTenant(ctx, auth0.EnsureTenantInput{
		TenantSlug:      input.TenantID,
		TenantName:      input.TenantName,
		AdminEmails:     input.AdminEmails,
		DefaultPassword: input.AdminPassword,
		ConnectionName:  p.Auth0Connection,
	})
	if err != nil {
		return nil, err
	}

	teamID, _, err := p.Konnect.EnsureTeamByName(ctx, input.TenantID, fmt.Sprintf("Tenant %s", input.TenantName), map[string]string{
		"tenant_id": input.TenantID,
	})
	if err != nil {
		return nil, err
	}

	systemAccountID, _, err := p.Konnect.EnsureSystemAccountByName(ctx, "tenant-"+input.TenantID+"-ingest", "Per-tenant ingest machine identity")
	if err != nil {
		return nil, err
	}
	roleSet := make(map[string]struct{}, len(requiredMeteringRoles)+1)
	roleOrder := make([]string, 0, len(requiredMeteringRoles)+1)
	for _, roleName := range requiredMeteringRoles {
		trimmedRole := strings.TrimSpace(roleName)
		if trimmedRole == "" {
			continue
		}
		if _, exists := roleSet[trimmedRole]; exists {
			continue
		}
		roleSet[trimmedRole] = struct{}{}
		roleOrder = append(roleOrder, trimmedRole)
	}
	if trimmedIngest := strings.TrimSpace(p.IngestRole); trimmedIngest != "" {
		if _, exists := roleSet[trimmedIngest]; !exists {
			roleSet[trimmedIngest] = struct{}{}
			roleOrder = append(roleOrder, trimmedIngest)
		}
	}
	for _, roleName := range roleOrder {
		if err := p.Konnect.EnsureRoleAssignedToSystemAccount(ctx, systemAccountID, roleName); err != nil {
			return nil, err
		}
	}
	if err := p.Konnect.EnsureSystemAccountInTeam(ctx, teamID, systemAccountID); err != nil {
		return nil, err
	}

	var token *konnect.SystemAccountToken
	existingToken, existingTokenErr := p.Registry.GetSPAT(input.TenantID)
	if existingTokenErr == nil && strings.TrimSpace(existingToken) != "" {
		token = &konnect.SystemAccountToken{
			AccountID: systemAccountID,
			TokenID:   "",
			Token:     strings.TrimSpace(existingToken),
		}
	} else {
		token, err = p.Konnect.CreateSystemAccountToken(ctx, systemAccountID, "tenant-"+input.TenantID+"-spat", time.Now().UTC().Add(p.SpatTTL))
		if err != nil {
			return nil, err
		}
	}

	openmeterProvisioner := p.OpenMeterFactory(token.Token)
	if err := openmeterProvisioner.EnsureDefaultCatalog(ctx); err != nil {
		return nil, err
	}
	if input.EnableSampleUser {
		_, err = openmeterProvisioner.Ensure(ctx, openmeter.ProvisionInput{
			TenantID:       strings.TrimSpace(input.TenantID),
			ClientID:       strings.TrimSpace(input.ClientID),
			ExternalUserID: strings.TrimSpace(input.ExternalUserID),
			DisplayName:    strings.TrimSpace(input.TenantID) + "/" + strings.TrimSpace(input.ClientID) + "/" + strings.TrimSpace(input.ExternalUserID),
		})
		if err != nil {
			return nil, err
		}
	}

	envFilePath, err := p.writeTenantEnv(input.TenantID, token.Token)
	if err != nil {
		return nil, err
	}

	existingTenantRecord, _ := p.Registry.Get(input.TenantID)
	systemTokenID := strings.TrimSpace(token.TokenID)
	if systemTokenID == "" && existingTenantRecord != nil {
		systemTokenID = strings.TrimSpace(existingTenantRecord.SystemTokenID)
	}

	if err := p.Registry.Upsert(registry.TenantRecord{
		TenantID:        input.TenantID,
		TenantName:      input.TenantName,
		Auth0OrgID:      auth0Result.OrganizationID,
		KonnectTeamID:   teamID,
		SystemAccountID: systemAccountID,
		SystemTokenID:   systemTokenID,
	}); err != nil {
		return nil, err
	}
	if strings.TrimSpace(input.ClientID) != "" {
		if err := p.Registry.UpsertApp(registry.TenantAppRecord{
			TenantID: strings.TrimSpace(input.TenantID),
			ClientID: strings.TrimSpace(input.ClientID),
		}); err != nil {
			return nil, err
		}
	}

	return &TenantProvisionResult{
		TenantID:          input.TenantID,
		Auth0Organization: auth0Result.OrganizationID,
		KonnectTeamID:     teamID,
		SystemAccountID:   systemAccountID,
		SystemAccountSPAT: token.Token,
		AdminUserIDs:      auth0Result.AdminUserIDs,
		EnvFile:           envFilePath,
	}, nil
}

func (p *Provisioner) EnsureCustomer(
	ctx context.Context,
	tenantID string,
	clientID string,
	externalUserID string,
) (*openmeter.ProvisionResult, error) {
	if p.DryRun {
		customerKey, _ := billingidentity.BuildCustomerKey(tenantID, clientID, externalUserID)
		return &openmeter.ProvisionResult{
			CustomerKey: customerKey,
			PlanKey:     "",
			Status:      "dry_run",
		}, nil
	}
	spat, err := p.Registry.GetSPAT(tenantID)
	if err != nil {
		return nil, err
	}
	provisioner := p.OpenMeterFactory(spat)
	if err := provisioner.EnsureDefaultCatalog(ctx); err != nil {
		return nil, err
	}
	return provisioner.Ensure(ctx, openmeter.ProvisionInput{
		TenantID:       strings.TrimSpace(tenantID),
		ClientID:       strings.TrimSpace(clientID),
		ExternalUserID: strings.TrimSpace(externalUserID),
		DisplayName:    strings.TrimSpace(tenantID) + "/" + strings.TrimSpace(clientID) + "/" + strings.TrimSpace(externalUserID),
	})
}

func (p *Provisioner) writeTenantEnv(tenantID, token string) (string, error) {
	if err := os.MkdirAll(p.DataDir, 0o755); err != nil {
		return "", fmt.Errorf("create tenant data dir: %w", err)
	}
	path := filepath.Join(p.DataDir, ".env."+strings.TrimSpace(tenantID))
	content := strings.Join([]string{
		"OPENMETER_API_KEY=" + strings.TrimSpace(token),
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", fmt.Errorf("write tenant env file: %w", err)
	}
	return path, nil
}

func (p *Provisioner) ListTenants(_ context.Context) ([]registry.TenantRecord, error) {
	return p.Registry.List()
}

func (p *Provisioner) GetTenant(_ context.Context, tenantID string) (*registry.TenantRecord, []registry.TenantAppRecord, error) {
	tenantRecord, err := p.Registry.Get(tenantID)
	if err != nil {
		return nil, nil, err
	}
	if tenantRecord == nil {
		return nil, nil, nil
	}
	appRecords, err := p.Registry.ListApps(tenantID)
	if err != nil {
		return nil, nil, err
	}
	return tenantRecord, appRecords, nil
}

func (p *Provisioner) RotateSPAT(ctx context.Context, tenantID string) (*konnect.SystemAccountToken, string, error) {
	tenantRecord, err := p.Registry.Get(tenantID)
	if err != nil {
		return nil, "", err
	}
	if tenantRecord == nil {
		return nil, "", fmt.Errorf("tenant %s not found", tenantID)
	}
	token, err := p.Konnect.CreateSystemAccountToken(
		ctx,
		tenantRecord.SystemAccountID,
		"tenant-"+strings.TrimSpace(tenantID)+"-spat-rotate",
		time.Now().UTC().Add(p.SpatTTL),
	)
	if err != nil {
		return nil, "", err
	}
	oldTokenID := strings.TrimSpace(tenantRecord.SystemTokenID)
	if oldTokenID != "" && oldTokenID != strings.TrimSpace(token.TokenID) {
		_ = p.Konnect.DeleteSystemAccountToken(ctx, tenantRecord.SystemAccountID, oldTokenID)
	}
	envFilePath, err := p.writeTenantEnv(tenantID, token.Token)
	if err != nil {
		return nil, "", err
	}
	tenantRecord.SystemTokenID = token.TokenID
	if err := p.Registry.Upsert(*tenantRecord); err != nil {
		return nil, "", err
	}
	return token, envFilePath, nil
}

func (p *Provisioner) ReadUsage(
	ctx context.Context,
	tenantID string,
	clientID string,
	externalUserID string,
	startDate string,
	endDate string,
) (*openmeter.UsageSummary, error) {
	spat, err := p.Registry.GetSPAT(tenantID)
	if err != nil {
		return nil, err
	}
	return openmeter.ReadUsage(ctx, p.OpenMeterURL, spat, tenantID, clientID, externalUserID, startDate, endDate)
}

func (p *Provisioner) ReadBalance(
	ctx context.Context,
	tenantID string,
	clientID string,
	externalUserID string,
	featureKey string,
) (*openmeter.BalanceSummary, error) {
	spat, err := p.Registry.GetSPAT(tenantID)
	if err != nil {
		return nil, err
	}
	return openmeter.ReadBalance(ctx, p.OpenMeterURL, spat, tenantID, clientID, externalUserID, featureKey)
}

func validateInput(input TenantProvisionInput) error {
	if strings.TrimSpace(input.TenantID) == "" {
		return fmt.Errorf("tenantId is required")
	}
	if strings.TrimSpace(input.TenantName) == "" {
		return fmt.Errorf("tenantName is required")
	}
	if len(input.AdminEmails) == 0 {
		return fmt.Errorf("adminEmails is required")
	}
	if strings.TrimSpace(input.AdminPassword) == "" {
		return fmt.Errorf("adminPassword is required")
	}
	return nil
}
