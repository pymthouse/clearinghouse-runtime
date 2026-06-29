package tenant

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/livepeer/clearinghouse/tenant-admin/internal/auth0"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/customerkey"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/konnect"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/openmeter"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/registry"
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
	TenantID          string `json:"tenantId"`
	Auth0Organization string `json:"auth0OrgId"`
	KonnectTeamID     string `json:"konnectTeamId"`
	IngestAccountID   string `json:"ingestAccountId"`
	IngestTokenID     string `json:"ingestTokenId"`
	// IngestSPAT is returned exactly once at creation and is never persisted.
	IngestSPAT   string   `json:"ingestSpat"`
	AdminUserIDs []string `json:"adminUsers"`
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
	GetTenantForClient(clientID string) (*registry.TenantRecord, error)
}

// Provisioner is the thin control plane. OpenMeter operations use a single
// service-held provisioner credential (clearinghouse-provisioner); per-tenant SPATs
// are ingest-only and are returned once, never stored.
type Provisioner struct {
	Auth0           Auth0Provisioner
	Konnect         KonnectProvisioner
	OpenMeter       OpenMeterProvisioner
	Registry        Registry
	IngestRole      string
	SpatTTL         time.Duration
	Auth0Connection string
	DryRun          bool
}

func (p *Provisioner) ProvisionTenant(ctx context.Context, input TenantProvisionInput) (*TenantProvisionResult, error) {
	if err := validateInput(input); err != nil {
		return nil, err
	}

	if p.DryRun {
		return &TenantProvisionResult{TenantID: input.TenantID}, nil
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

	ingestAccountID, _, err := p.Konnect.EnsureSystemAccountByName(ctx, ingestAccountName(input.TenantID), "Per-tenant ingest machine identity")
	if err != nil {
		return nil, err
	}
	// Ingest-only role binding. Reader/admin roles are intentionally not assigned to
	// the tenant's machine identity (see konnect/systemaccounts.go security gate).
	if err := p.Konnect.EnsureRoleAssignedToSystemAccount(ctx, ingestAccountID, p.ingestRole()); err != nil {
		return nil, err
	}
	if err := p.Konnect.EnsureSystemAccountInTeam(ctx, teamID, ingestAccountID); err != nil {
		return nil, err
	}

	token, err := p.Konnect.CreateSystemAccountToken(ctx, ingestAccountID, ingestAccountName(input.TenantID)+"-spat", time.Now().UTC().Add(p.SpatTTL))
	if err != nil {
		return nil, err
	}

	// OpenMeter provisioning uses the service-held provisioner credential, not the
	// tenant SPAT, so no per-tenant token ever needs to be stored for reads/writes here.
	if err := p.OpenMeter.EnsureDefaultCatalog(ctx); err != nil {
		return nil, err
	}
	if input.EnableSampleUser {
		// DisplayName left empty -> defaults to the clientId:externalUserId customer key.
		if _, err := p.OpenMeter.Ensure(ctx, openmeter.ProvisionInput{
			TenantID:       strings.TrimSpace(input.TenantID),
			ClientID:       strings.TrimSpace(input.ClientID),
			ExternalUserID: strings.TrimSpace(input.ExternalUserID),
		}); err != nil {
			return nil, err
		}
	}

	if err := p.Registry.Upsert(registry.TenantRecord{
		TenantID:        input.TenantID,
		TenantName:      input.TenantName,
		Auth0OrgID:      auth0Result.OrganizationID,
		KonnectTeamID:   teamID,
		IngestAccountID: ingestAccountID,
		IngestTokenID:   strings.TrimSpace(token.TokenID),
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
		IngestAccountID:   ingestAccountID,
		IngestTokenID:     strings.TrimSpace(token.TokenID),
		IngestSPAT:        token.Token,
		AdminUserIDs:      auth0Result.AdminUserIDs,
	}, nil
}

// EnsureCustomer ensures the OpenMeter customer + subscription for an (app, end-user)
// identity, keyed by clientId:externalUserId (builder-sdk/pymthouse format). It is the
// proactive "know the customer before service" path, called synchronously from the
// identity gate. It does NOT (re)ensure the global catalog — that is a provisioning-time
// concern (see ProvisionTenant) — so it stays cheap on the hot path.
//
// tenantId is resolved from the registry (by clientId) for labelling only; it is not part
// of the customer key.
func (p *Provisioner) EnsureCustomer(ctx context.Context, clientID, externalUserID string) (*openmeter.ProvisionResult, error) {
	clientID = strings.TrimSpace(clientID)
	externalUserID = strings.TrimSpace(externalUserID)
	if p.DryRun {
		customerKey, err := customerkey.Build(clientID, externalUserID)
		if err != nil {
			return nil, err
		}
		return &openmeter.ProvisionResult{CustomerKey: customerKey, Status: "dry_run"}, nil
	}
	tenantID := ""
	if record, err := p.Registry.GetTenantForClient(clientID); err == nil && record != nil {
		tenantID = record.TenantID
	}
	return p.OpenMeter.Ensure(ctx, openmeter.ProvisionInput{
		TenantID:       tenantID,
		ClientID:       clientID,
		ExternalUserID: externalUserID,
	})
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

func (p *Provisioner) RegisterApp(_ context.Context, tenantID, clientID string) error {
	tenantRecord, err := p.Registry.Get(tenantID)
	if err != nil {
		return err
	}
	if tenantRecord == nil {
		return fmt.Errorf("tenant %s not found", tenantID)
	}
	return p.Registry.UpsertApp(registry.TenantAppRecord{
		TenantID: strings.TrimSpace(tenantID),
		ClientID: strings.TrimSpace(clientID),
	})
}

// CreateIngestToken issues a fresh ingest SPAT for the tenant. The token value is
// returned once; only its ID is persisted.
func (p *Provisioner) CreateIngestToken(ctx context.Context, tenantID string) (*konnect.SystemAccountToken, error) {
	tenantRecord, err := p.Registry.Get(tenantID)
	if err != nil {
		return nil, err
	}
	if tenantRecord == nil {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}
	token, err := p.Konnect.CreateSystemAccountToken(
		ctx,
		tenantRecord.IngestAccountID,
		ingestAccountName(tenantID)+"-spat",
		time.Now().UTC().Add(p.SpatTTL),
	)
	if err != nil {
		return nil, err
	}
	tenantRecord.IngestTokenID = strings.TrimSpace(token.TokenID)
	if err := p.Registry.Upsert(*tenantRecord); err != nil {
		return nil, err
	}
	return token, nil
}

// RotateIngestToken issues a new ingest SPAT and revokes the prior one. If oldTokenID
// is empty the currently stored token ID is revoked instead.
func (p *Provisioner) RotateIngestToken(ctx context.Context, tenantID, oldTokenID string) (*konnect.SystemAccountToken, error) {
	tenantRecord, err := p.Registry.Get(tenantID)
	if err != nil {
		return nil, err
	}
	if tenantRecord == nil {
		return nil, fmt.Errorf("tenant %s not found", tenantID)
	}
	token, err := p.Konnect.CreateSystemAccountToken(
		ctx,
		tenantRecord.IngestAccountID,
		ingestAccountName(tenantID)+"-spat-rotate",
		time.Now().UTC().Add(p.SpatTTL),
	)
	if err != nil {
		return nil, err
	}
	revokeID := strings.TrimSpace(oldTokenID)
	if revokeID == "" {
		revokeID = strings.TrimSpace(tenantRecord.IngestTokenID)
	}
	if revokeID != "" && revokeID != strings.TrimSpace(token.TokenID) {
		_ = p.Konnect.DeleteSystemAccountToken(ctx, tenantRecord.IngestAccountID, revokeID)
	}
	tenantRecord.IngestTokenID = strings.TrimSpace(token.TokenID)
	if err := p.Registry.Upsert(*tenantRecord); err != nil {
		return nil, err
	}
	return token, nil
}

func (p *Provisioner) ingestRole() string {
	role := strings.TrimSpace(p.IngestRole)
	if role == "" {
		return "Ingest"
	}
	return role
}

func ingestAccountName(tenantID string) string {
	return "tenant-" + strings.TrimSpace(tenantID) + "-ingest"
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
