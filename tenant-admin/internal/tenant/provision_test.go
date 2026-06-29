package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/livepeer/clearinghouse/tenant-admin/internal/auth0"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/konnect"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/openmeter"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/registry"
)

type fakeAuth0 struct{}

func (f fakeAuth0) EnsureTenant(_ context.Context, _ auth0.EnsureTenantInput) (*auth0.EnsureTenantResult, error) {
	return &auth0.EnsureTenantResult{
		OrganizationID: "org_123",
		AdminUserIDs:   []string{"auth0|abc"},
	}, nil
}

type fakeKonnect struct {
	assignedRoles []string
	deletedTokens []string
	createdTokens int
}

func (f *fakeKonnect) EnsureTeamByName(_ context.Context, _ string, _ string, _ map[string]string) (string, bool, error) {
	return "team_123", true, nil
}
func (f *fakeKonnect) EnsureSystemAccountByName(_ context.Context, _ string, _ string) (string, bool, error) {
	return "sa_123", true, nil
}
func (f *fakeKonnect) CreateSystemAccountToken(_ context.Context, _ string, name string, expiresAt time.Time) (*konnect.SystemAccountToken, error) {
	f.createdTokens++
	return &konnect.SystemAccountToken{
		AccountID: "sa_123",
		TokenID:   "tok_" + name,
		Token:     "spat_secret",
		ExpiresAt: expiresAt,
	}, nil
}
func (f *fakeKonnect) DeleteSystemAccountToken(_ context.Context, _ string, tokenID string) error {
	f.deletedTokens = append(f.deletedTokens, tokenID)
	return nil
}
func (f *fakeKonnect) EnsureSystemAccountInTeam(_ context.Context, _ string, _ string) error {
	return nil
}
func (f *fakeKonnect) EnsureRoleAssignedToSystemAccount(_ context.Context, _ string, roleName string) error {
	f.assignedRoles = append(f.assignedRoles, roleName)
	return nil
}

type fakeOpenMeter struct {
	catalogEnsured bool
	customers      int
}

func (f *fakeOpenMeter) EnsureDefaultCatalog(_ context.Context) error {
	f.catalogEnsured = true
	return nil
}
func (f *fakeOpenMeter) Ensure(_ context.Context, input openmeter.ProvisionInput) (*openmeter.ProvisionResult, error) {
	f.customers++
	return &openmeter.ProvisionResult{CustomerKey: input.ClientID + ":" + input.ExternalUserID}, nil
}

type fakeRegistry struct {
	record registry.TenantRecord
	apps   []registry.TenantAppRecord
}

func (f *fakeRegistry) Upsert(record registry.TenantRecord) error {
	f.record = record
	return nil
}
func (f *fakeRegistry) UpsertApp(record registry.TenantAppRecord) error {
	f.apps = append(f.apps, record)
	return nil
}
func (f *fakeRegistry) Get(tenantID string) (*registry.TenantRecord, error) {
	if tenantID == f.record.TenantID {
		copyRecord := f.record
		return &copyRecord, nil
	}
	return nil, nil
}
func (f *fakeRegistry) List() ([]registry.TenantRecord, error) {
	if f.record.TenantID == "" {
		return []registry.TenantRecord{}, nil
	}
	return []registry.TenantRecord{f.record}, nil
}
func (f *fakeRegistry) ListApps(tenantID string) ([]registry.TenantAppRecord, error) {
	filtered := make([]registry.TenantAppRecord, 0, len(f.apps))
	for _, app := range f.apps {
		if app.TenantID == tenantID {
			filtered = append(filtered, app)
		}
	}
	return filtered, nil
}
func (f *fakeRegistry) GetTenantForClient(clientID string) (*registry.TenantRecord, error) {
	for _, app := range f.apps {
		if app.ClientID == clientID {
			return f.Get(app.TenantID)
		}
	}
	return nil, nil
}

func newProvisioner(reg *fakeRegistry, k *fakeKonnect, om *fakeOpenMeter) *Provisioner {
	return &Provisioner{
		Auth0:           fakeAuth0{},
		Konnect:         k,
		OpenMeter:       om,
		Registry:        reg,
		IngestRole:      "Ingest",
		SpatTTL:         24 * time.Hour,
		Auth0Connection: "Username-Password-Authentication",
	}
}

func TestProvisionTenantWritesRegistryAndReturnsSPATOnce(t *testing.T) {
	reg := &fakeRegistry{}
	k := &fakeKonnect{}
	om := &fakeOpenMeter{}
	p := newProvisioner(reg, k, om)

	result, err := p.ProvisionTenant(context.Background(), TenantProvisionInput{
		TenantID:         "acme",
		TenantName:       "Acme",
		AdminEmails:      []string{"admin@acme.com"},
		AdminPassword:    "Password123!",
		ClientID:         "app_acme",
		ExternalUserID:   "user_1",
		EnableSampleUser: true,
	})
	if err != nil {
		t.Fatalf("ProvisionTenant failed: %v", err)
	}
	if result.KonnectTeamID != "team_123" {
		t.Fatalf("expected team_123, got %s", result.KonnectTeamID)
	}
	if result.IngestSPAT != "spat_secret" {
		t.Fatalf("expected one-time SPAT to be returned, got %q", result.IngestSPAT)
	}
	// Only the ingest role must be assigned to the tenant machine identity.
	if len(k.assignedRoles) != 1 || k.assignedRoles[0] != "Ingest" {
		t.Fatalf("expected exactly [Ingest], got %v", k.assignedRoles)
	}
	if !om.catalogEnsured || om.customers != 1 {
		t.Fatalf("expected catalog ensured and 1 sample customer, got ensured=%v customers=%d", om.catalogEnsured, om.customers)
	}
	// The registry must persist the token ID but never the token value.
	if reg.record.IngestTokenID == "" {
		t.Fatalf("expected ingest token ID persisted")
	}
	if reg.record.TenantID != "acme" {
		t.Fatalf("expected registry tenant acme, got %s", reg.record.TenantID)
	}
}

func TestRotateIngestTokenRevokesPrevious(t *testing.T) {
	reg := &fakeRegistry{record: registry.TenantRecord{TenantID: "acme", IngestAccountID: "sa_123", IngestTokenID: "tok_old"}}
	k := &fakeKonnect{}
	p := newProvisioner(reg, k, &fakeOpenMeter{})

	token, err := p.RotateIngestToken(context.Background(), "acme", "")
	if err != nil {
		t.Fatalf("RotateIngestToken failed: %v", err)
	}
	if token.Token != "spat_secret" {
		t.Fatalf("expected new SPAT returned")
	}
	if len(k.deletedTokens) != 1 || k.deletedTokens[0] != "tok_old" {
		t.Fatalf("expected previous token tok_old revoked, got %v", k.deletedTokens)
	}
	if reg.record.IngestTokenID == "tok_old" {
		t.Fatalf("expected stored token ID to advance to the new token")
	}
}

func TestProvisionTenantDryRun(t *testing.T) {
	p := &Provisioner{DryRun: true}
	result, err := p.ProvisionTenant(context.Background(), TenantProvisionInput{
		TenantID:      "dry-run",
		TenantName:    "Dry Run",
		AdminEmails:   []string{"admin@example.com"},
		AdminPassword: "Password123!",
	})
	if err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
	if result.TenantID != "dry-run" {
		t.Fatalf("expected dry-run tenant ID")
	}
}
