package config

import (
	"os"
	"path/filepath"
	"testing"
)

const sampleYAML = `
tenant:
  defaultAudience: livepeer-clearinghouse
  deviceFlow: { charset: base20, mask: "****-****" }
  enabledLocales: [en]
resourceServers:
  - name: Livepeer Clearinghouse API
    identifier: livepeer-clearinghouse
    signingAlg: RS256
    tokenLifetime: 86400
    allowOfflineAccess: true
    scopes:
      - { value: "sign:job", description: "Sign tickets" }
      - { value: "users:token", description: "Per-user tokens" }
apps:
  - name: Demo App
    audience: livepeer-clearinghouse
    public:
      grantScopes: ["sign:job"]
    m2m:
      grantScopes: ["users:write", "users:token", "device:approve", "sign:job"]
  - name: Per-User App
    audience: livepeer-clearinghouse
    public:
      grantScopes: ["sign:job", "users:token"]
    m2m:
      grantScopes: ["users:write", "users:token", "device:approve"]
`

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "auth0.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadCatalog(t *testing.T) {
	cat, err := LoadCatalog(writeTemp(t, sampleYAML))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}

	if got := len(cat.ResourceServers); got != 1 {
		t.Fatalf("resource servers = %d, want 1", got)
	}
	if cat.ResourceServers[0].Identifier != "livepeer-clearinghouse" {
		t.Errorf("identifier = %q", cat.ResourceServers[0].Identifier)
	}
	if !cat.ResourceServers[0].AllowOfflineAccess {
		t.Error("allowOfflineAccess should be true")
	}
	if cat.Tenant.DeviceFlow.Charset != "base20" {
		t.Errorf("device flow charset = %q", cat.Tenant.DeviceFlow.Charset)
	}

	if got := len(cat.Apps); got != 2 {
		t.Fatalf("apps = %d, want 2", got)
	}
	perUser := cat.Apps[1]
	if perUser.Name != "Per-User App" {
		t.Errorf("apps[1].name = %q", perUser.Name)
	}
	if len(perUser.Public.GrantScopes) != 2 || perUser.Public.GrantScopes[1] != "users:token" {
		t.Errorf("per-user public grant scopes = %v", perUser.Public.GrantScopes)
	}
}

func TestLoadCatalogValidationUnmatchedAudience(t *testing.T) {
	bad := `
resourceServers:
  - { name: API, identifier: aud-a }
apps:
  - name: X
    audience: aud-b
    public: { grantScopes: ["sign:job"] }
    m2m: { grantScopes: ["users:write"] }
`
	if _, err := LoadCatalog(writeTemp(t, bad)); err == nil {
		t.Fatal("expected validation error for audience with no matching resource server")
	}
}

func TestLoadCatalogValidationNoResourceServers(t *testing.T) {
	bad := `
apps:
  - name: X
    audience: aud-a
    public: { grantScopes: ["sign:job"] }
    m2m: { grantScopes: ["users:write"] }
`
	if _, err := LoadCatalog(writeTemp(t, bad)); err == nil {
		t.Fatal("expected validation error when no resourceServers are defined")
	}
}
