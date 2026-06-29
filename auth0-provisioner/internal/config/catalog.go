package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Catalog is the declarative Auth0 tenant slice — the data source of truth
// (analog of the OpenMeter provisioner's catalog.json). It describes one tenant,
// one or more resource servers (APIs), and N public/confidential client pairs
// modeling pymthouse's OIDC issuer design.
type Catalog struct {
	Tenant          TenantConfig           `yaml:"tenant"`
	ResourceServers []ResourceServerConfig `yaml:"resourceServers"`
	Apps            []AppConfig            `yaml:"apps"`
}

// TenantConfig carries tenant-wide settings, chiefly the RFC 8628 device-flow
// enablement (a default audience lets the device authorization endpoint mint API
// tokens without an explicit audience parameter).
type TenantConfig struct {
	DefaultAudience  string           `yaml:"defaultAudience"`
	DefaultDirectory string           `yaml:"defaultDirectory"`
	DeviceFlow       DeviceFlowConfig `yaml:"deviceFlow"`
	EnabledLocales   []string         `yaml:"enabledLocales"`
}

// DeviceFlowConfig formats the user_code shown during device authorization.
type DeviceFlowConfig struct {
	Charset string `yaml:"charset"`
	Mask    string `yaml:"mask"`
}

// ResourceServerConfig is an Auth0 API (resource server). Its identifier is the
// JWT audience, and its scopes are the full set callable across all client pairs.
type ResourceServerConfig struct {
	Name                                      string        `yaml:"name"`
	Identifier                                string        `yaml:"identifier"`
	SigningAlg                                string        `yaml:"signingAlg"`
	TokenLifetime                             int           `yaml:"tokenLifetime"`
	AllowOfflineAccess                        bool          `yaml:"allowOfflineAccess"`
	SkipConsentForVerifiableFirstPartyClients bool          `yaml:"skipConsentForVerifiableFirstPartyClients"`
	Scopes                                    []ScopeConfig `yaml:"scopes"`
}

// ScopeConfig is a single resource-server scope definition.
type ScopeConfig struct {
	Value       string `yaml:"value"`
	Description string `yaml:"description"`
}

// AppConfig is one interactive app: a public/native client (device flow) paired
// with a confidential/M2M client, both granted against the same audience.
type AppConfig struct {
	Name     string             `yaml:"name"`
	Audience string             `yaml:"audience"`
	Public   PublicClientConfig `yaml:"public"`
	M2M      M2MClientConfig    `yaml:"m2m"`
}

// PublicClientConfig is the native, secret-less device-flow client. Its
// grantScopes drive end-user token claims and billing mode (presence of
// users:token ⇒ per-user billing).
type PublicClientConfig struct {
	GrantScopes      []string `yaml:"grantScopes"`
	Callbacks        []string `yaml:"callbacks"`
	InitiateLoginURI string   `yaml:"initiateLoginUri"`
}

// M2MClientConfig is the confidential client. Its grantScopes gate server-side
// Builder API and RFC 8693 token-exchange calls.
type M2MClientConfig struct {
	GrantScopes []string `yaml:"grantScopes"`
}

// LoadCatalog reads and validates the declarative Auth0 config.
func LoadCatalog(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading auth0 config %s: %w", path, err)
	}
	var c Catalog
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parsing auth0 config %s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("auth0 config %s: %w", path, err)
	}
	return &c, nil
}

func (c *Catalog) validate() error {
	if len(c.ResourceServers) == 0 {
		return fmt.Errorf("at least one resourceServers entry is required")
	}
	known := make(map[string]bool, len(c.ResourceServers))
	for i, rs := range c.ResourceServers {
		if rs.Identifier == "" {
			return fmt.Errorf("resourceServers[%d] missing identifier", i)
		}
		known[rs.Identifier] = true
	}
	if len(c.Apps) == 0 {
		return fmt.Errorf("at least one apps entry is required")
	}
	for i, app := range c.Apps {
		if app.Name == "" {
			return fmt.Errorf("apps[%d] missing name", i)
		}
		if app.Audience == "" {
			return fmt.Errorf("apps[%d] (%s) missing audience", i, app.Name)
		}
		if !known[app.Audience] {
			return fmt.Errorf("apps[%d] (%s) audience %q has no matching resourceServers identifier", i, app.Name, app.Audience)
		}
	}
	return nil
}
