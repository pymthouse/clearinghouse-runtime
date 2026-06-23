package tenant

import (
	"context"
	"testing"
	"time"

	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/auth0"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/konnect"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/openmeter"
	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/registry"
)

type fakeAuth0 struct{}

func (f fakeAuth0) EnsureTenant(_ context.Context, _ auth0.EnsureTenantInput) (*auth0.EnsureTenantResult, error) {
	return &auth0.EnsureTenantResult{
		OrganizationID: "org_123",
		AdminUserIDs:   []string{"auth0|abc"},
	}, nil
}

type fakeKonnect struct{}

func (f fakeKonnect) EnsureTeamByName(_ context.Context, _ string, _ string, _ map[string]string) (string, bool, error) {
	return "team_123", true, nil
}
func (f fakeKonnect) EnsureSystemAccountByName(_ context.Context, _ string, _ string) (string, bool, error) {
	return "sa_123", true, nil
}
func (f fakeKonnect) CreateSystemAccountToken(_ context.Context, _ string, _ string, expiresAt time.Time) (*konnect.SystemAccountToken, error) {
	return &konnect.SystemAccountToken{
		AccountID: "sa_123",
		TokenID:   "tok_123",
		Token:     "spat_abc",
		ExpiresAt: expiresAt,
	}, nil
}
func (f fakeKonnect) DeleteSystemAccountToken(_ context.Context, _ string, _ string) error {
	return nil
}
func (f fakeKonnect) EnsureSystemAccountInTeam(_ context.Context, _ string, _ string) error {
	return nil
}
func (f fakeKonnect) EnsureRoleAssignedToSystemAccount(_ context.Context, _ string, _ string) error {
	return nil
}

type fakeOpenMeter struct{}

func (f fakeOpenMeter) EnsureDefaultCatalog(_ context.Context) error {
	return nil
}

func (f fakeOpenMeter) Ensure(_ context.Context, input openmeter.ProvisionInput) (*openmeter.ProvisionResult, error) {
	return &openmeter.ProvisionResult{
		CustomerKey: input.ClientID + ":" + input.ExternalUserID,
	}, nil
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

func (f *fakeRegistry) GetSPAT(_ string) (string, error) {
	return "spat_abc", nil
}

func (f *fakeRegistry) GetTenantForClient(clientID string) (*registry.TenantRecord, error) {
	for _, app := range f.apps {
		if app.ClientID == clientID {
			return f.Get(app.TenantID)
		}
	}
	return nil, nil
}

func TestProvisionTenantWritesRegistry(t *testing.T) {
	reg := &fakeRegistry{}
	p := &Provisioner{
		Auth0:   fakeAuth0{},
		Konnect: fakeKonnect{},
		OpenMeterFactory: func(_ string) OpenMeterProvisioner {
			return fakeOpenMeter{}
		},
		Registry:        reg,
		DataDir:         t.TempDir(),
		IngestRole:      "Ingest",
		SpatTTL:         24 * time.Hour,
		Auth0Connection: "Username-Password-Authentication",
	}

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
	if reg.record.TenantID != "acme" {
		t.Fatalf("expected registry tenant acme, got %s", reg.record.TenantID)
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
