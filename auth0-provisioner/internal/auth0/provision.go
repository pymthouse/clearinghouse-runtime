// Package auth0 idempotently reconciles an Auth0 tenant slice — tenant device-flow
// settings, resource server(s), and public/confidential client pairs — from the
// declarative config. It mirrors the OpenMeter provisioner's "fetch → check →
// create-or-update, never blind-create" approach so re-runs are safe.
//
// It uses the idiomatic github.com/auth0/go-auth0 (v1) management SDK: plain
// pointer-field structs, the auth0.String/Bool/Int helpers, and server-side list
// filters.
package auth0

import (
	"context"
	"fmt"

	"github.com/auth0/go-auth0"
	"github.com/auth0/go-auth0/management"

	"github.com/livepeer/clearinghouse/auth0-provisioner/internal/config"
)

// OIDC grant types provisioned on the client pairs.
const (
	deviceCodeGrant        = "urn:ietf:params:oauth:grant-type:device_code" // RFC 8628
	refreshTokenGrant      = "refresh_token"
	clientCredentialsGrant = "client_credentials"
)

// clientPageSize bounds each page when scanning clients for a name match.
const clientPageSize = 100

// Runtime carries the Auth0 Management API connection details.
type Runtime struct {
	Domain           string
	MgmtClientID     string
	MgmtClientSecret string
}

// Logf is an optional progress sink.
type Logf func(format string, args ...any)

// AppResult is the provisioned identity of one public/confidential pair.
type AppResult struct {
	Name            string
	Audience        string
	PublicClientID  string
	M2MClientID     string
	M2MClientSecret string
}

// Result is the full provisioning outcome for the tenant.
type Result struct {
	Domain  string
	Issuer  string
	JwksURL string
	Apps    []AppResult
}

// Provision reconciles the whole catalog against the Auth0 tenant and returns the
// resulting client identities (public id, M2M id + secret) per app.
func Provision(ctx context.Context, rt Runtime, cat *config.Catalog, logf Logf) (*Result, error) {
	if logf == nil {
		logf = func(string, ...any) {}
	}

	m, err := management.New(rt.Domain, management.WithClientCredentials(ctx, rt.MgmtClientID, rt.MgmtClientSecret))
	if err != nil {
		return nil, fmt.Errorf("creating Auth0 management client: %w", err)
	}

	// 1. Tenant settings — enable RFC 8628 device flow at the tenant scope.
	if err := reconcileTenant(ctx, m, cat.Tenant, logf); err != nil {
		return nil, err
	}

	// 2. Resource servers (APIs).
	for _, rs := range cat.ResourceServers {
		if err := reconcileResourceServer(ctx, m, rs, logf); err != nil {
			return nil, err
		}
	}

	// 3 + 4. Client pairs and their grants.
	result := &Result{
		Domain:  rt.Domain,
		Issuer:  fmt.Sprintf("https://%s/", rt.Domain),
		JwksURL: fmt.Sprintf("https://%s/.well-known/jwks.json", rt.Domain),
	}
	for _, app := range cat.Apps {
		ar, err := reconcileApp(ctx, m, app, logf)
		if err != nil {
			return nil, err
		}
		result.Apps = append(result.Apps, *ar)
	}

	return result, nil
}

func reconcileTenant(ctx context.Context, m *management.Management, t config.TenantConfig, logf Logf) error {
	patch := &management.Tenant{}
	changed := false

	if t.DefaultAudience != "" {
		patch.DefaultAudience = auth0.String(t.DefaultAudience)
		changed = true
	}
	if t.DefaultDirectory != "" {
		patch.DefaultDirectory = auth0.String(t.DefaultDirectory)
		changed = true
	}
	if t.DeviceFlow.Charset != "" || t.DeviceFlow.Mask != "" {
		df := &management.TenantDeviceFlow{}
		if t.DeviceFlow.Charset != "" {
			df.Charset = auth0.String(t.DeviceFlow.Charset)
		}
		if t.DeviceFlow.Mask != "" {
			df.Mask = auth0.String(t.DeviceFlow.Mask)
		}
		patch.DeviceFlow = df
		changed = true
	}
	if len(t.EnabledLocales) > 0 {
		locales := append([]string(nil), t.EnabledLocales...)
		patch.EnabledLocales = &locales
		changed = true
	}

	if !changed {
		logf("tenant: no settings to apply")
		return nil
	}
	if err := m.Tenant.Update(ctx, patch); err != nil {
		return fmt.Errorf("updating tenant settings: %w", err)
	}
	logf("tenant: settings applied (default_audience=%q device_flow_charset=%q)", t.DefaultAudience, t.DeviceFlow.Charset)
	return nil
}

func reconcileResourceServer(ctx context.Context, m *management.Management, rs config.ResourceServerConfig, logf Logf) error {
	scopes := make([]management.ResourceServerScope, 0, len(rs.Scopes))
	for _, s := range rs.Scopes {
		scope := management.ResourceServerScope{Value: auth0.String(s.Value)}
		if s.Description != "" {
			scope.Description = auth0.String(s.Description)
		}
		scopes = append(scopes, scope)
	}

	desired := &management.ResourceServer{
		Identifier:         auth0.String(rs.Identifier),
		Scopes:             &scopes,
		AllowOfflineAccess: auth0.Bool(rs.AllowOfflineAccess),
		SkipConsentForVerifiableFirstPartyClients: auth0.Bool(rs.SkipConsentForVerifiableFirstPartyClients),
	}
	if rs.Name != "" {
		desired.Name = auth0.String(rs.Name)
	}
	if rs.SigningAlg != "" {
		desired.SigningAlgorithm = auth0.String(rs.SigningAlg)
	}
	if rs.TokenLifetime > 0 {
		desired.TokenLifetime = auth0.Int(rs.TokenLifetime)
	}

	existing, err := findResourceServer(ctx, m, rs.Identifier)
	if err != nil {
		return err
	}
	if existing == nil {
		if err := m.ResourceServer.Create(ctx, desired); err != nil {
			return fmt.Errorf("creating resource server %s: %w", rs.Identifier, err)
		}
		logf("resource server %s: created", rs.Identifier)
		return nil
	}

	// Identifier is immutable; updates address the existing resource by id.
	desired.Identifier = nil
	if err := m.ResourceServer.Update(ctx, existing.GetID(), desired); err != nil {
		return fmt.Errorf("updating resource server %s: %w", rs.Identifier, err)
	}
	logf("resource server %s: updated", rs.Identifier)
	return nil
}

// clientSpec captures the role-specific differences between the public (native,
// device-flow) and confidential (M2M) clients so a single ensureClient handles both.
type clientSpec struct {
	name             string
	description      string
	appType          string
	authMethod       string
	grantTypes       []string
	isFirstParty     bool
	callbacks        []string
	initiateLoginURI string
	// readSecret reports whether the client's secret should be returned (M2M only).
	readSecret bool
}

func reconcileApp(ctx context.Context, m *management.Management, app config.AppConfig, logf Logf) (*AppResult, error) {
	publicID, _, err := ensureClient(ctx, m, clientSpec{
		name:             fmt.Sprintf("%s Public", app.Name),
		description:      "Public client for SDK, CLI, and device authorization flow (RFC 8628)",
		appType:          "native",
		authMethod:       "none",
		grantTypes:       []string{deviceCodeGrant, refreshTokenGrant},
		isFirstParty:     true,
		callbacks:        app.Public.Callbacks,
		initiateLoginURI: app.Public.InitiateLoginURI,
	}, logf)
	if err != nil {
		return nil, err
	}

	m2mID, m2mSecret, err := ensureClient(ctx, m, clientSpec{
		name:        fmt.Sprintf("%s M2M", app.Name),
		description: "Confidential client for server-side Builder API and RFC 8693 token exchange",
		appType:     "non_interactive",
		authMethod:  "client_secret_post",
		grantTypes:  []string{clientCredentialsGrant},
		readSecret:  true,
	}, logf)
	if err != nil {
		return nil, err
	}

	if err := ensureClientGrant(ctx, m, publicID, app.Audience, app.Public.GrantScopes, "public", logf); err != nil {
		return nil, err
	}
	if err := ensureClientGrant(ctx, m, m2mID, app.Audience, app.M2M.GrantScopes, "m2m", logf); err != nil {
		return nil, err
	}

	return &AppResult{
		Name:            app.Name,
		Audience:        app.Audience,
		PublicClientID:  publicID,
		M2MClientID:     m2mID,
		M2MClientSecret: m2mSecret,
	}, nil
}

// ensureClient reconciles one client (create-or-update by name) and returns its id
// and — for confidential clients — its secret. On create the secret is returned
// directly; on re-run it is read back (requires read:client_keys).
func ensureClient(ctx context.Context, m *management.Management, spec clientSpec, logf Logf) (clientID, secret string, err error) {
	grants := append([]string(nil), spec.grantTypes...)
	desired := &management.Client{
		Name:                    auth0.String(spec.name),
		Description:             auth0.String(spec.description),
		AppType:                 auth0.String(spec.appType),
		TokenEndpointAuthMethod: auth0.String(spec.authMethod),
		OIDCConformant:          auth0.Bool(true),
		GrantTypes:              &grants,
	}
	if spec.isFirstParty {
		desired.IsFirstParty = auth0.Bool(true)
	}
	if len(spec.callbacks) > 0 {
		cbs := append([]string(nil), spec.callbacks...)
		desired.Callbacks = &cbs
	}
	if spec.initiateLoginURI != "" {
		desired.InitiateLoginURI = auth0.String(spec.initiateLoginURI)
	}

	existing, err := findClientByName(ctx, m, spec.name)
	if err != nil {
		return "", "", err
	}

	if existing == nil {
		if err := m.Client.Create(ctx, desired); err != nil {
			return "", "", fmt.Errorf("creating client %q: %w", spec.name, err)
		}
		logf("client %q: created (%s)", spec.name, spec.appType)
		if spec.readSecret && desired.GetClientSecret() == "" {
			return "", "", fmt.Errorf("client %q created without a client_secret", spec.name)
		}
		return desired.GetClientID(), desired.GetClientSecret(), nil
	}

	id := existing.GetClientID()
	if err := m.Client.Update(ctx, id, desired); err != nil {
		return "", "", fmt.Errorf("updating client %q: %w", spec.name, err)
	}
	logf("client %q: exists (%s)", spec.name, spec.appType)

	if !spec.readSecret {
		return id, "", nil
	}
	got, err := m.Client.Read(ctx, id)
	if err != nil {
		return "", "", fmt.Errorf("reading client %q: %w", spec.name, err)
	}
	if got.GetClientSecret() == "" {
		return "", "", fmt.Errorf("client %q secret unavailable — Management token needs read:client_keys", spec.name)
	}
	return id, got.GetClientSecret(), nil
}

// ensureClientGrant reconciles a client→audience grant with the given scopes.
func ensureClientGrant(ctx context.Context, m *management.Management, clientID, audience string, scopes []string, role string, logf Logf) error {
	existing, err := findClientGrant(ctx, m, clientID, audience)
	if err != nil {
		return err
	}

	wanted := append([]string(nil), scopes...)
	if existing == nil {
		if err := m.ClientGrant.Create(ctx, &management.ClientGrant{
			ClientID: auth0.String(clientID),
			Audience: auth0.String(audience),
			Scope:    &wanted,
		}); err != nil {
			return fmt.Errorf("creating %s client grant (%s → %s): %w", role, clientID, audience, err)
		}
		logf("grant %s: created (%s → %s) scopes=%v", role, clientID, audience, scopes)
		return nil
	}

	if err := m.ClientGrant.Update(ctx, existing.GetID(), &management.ClientGrant{Scope: &wanted}); err != nil {
		return fmt.Errorf("updating %s client grant (%s → %s): %w", role, clientID, audience, err)
	}
	logf("grant %s: updated (%s → %s) scopes=%v", role, clientID, audience, scopes)
	return nil
}

func findResourceServer(ctx context.Context, m *management.Management, identifier string) (*management.ResourceServer, error) {
	list, err := m.ResourceServer.List(ctx, management.Parameter("identifier", identifier))
	if err != nil {
		return nil, fmt.Errorf("listing resource servers: %w", err)
	}
	for _, rs := range list.ResourceServers {
		if rs.GetIdentifier() == identifier {
			return rs, nil
		}
	}
	return nil, nil
}

func findClientByName(ctx context.Context, m *management.Management, name string) (*management.Client, error) {
	for page := 0; ; page++ {
		list, err := m.Client.List(ctx, management.Page(page), management.PerPage(clientPageSize), management.IncludeFields("client_id", "name"))
		if err != nil {
			return nil, fmt.Errorf("listing clients: %w", err)
		}
		for _, c := range list.Clients {
			if c.GetName() == name {
				return c, nil
			}
		}
		if !list.HasNext() {
			return nil, nil
		}
	}
}

func findClientGrant(ctx context.Context, m *management.Management, clientID, audience string) (*management.ClientGrant, error) {
	list, err := m.ClientGrant.List(ctx,
		management.Parameter("client_id", clientID),
		management.Parameter("audience", audience),
	)
	if err != nil {
		return nil, fmt.Errorf("listing client grants: %w", err)
	}
	for _, g := range list.ClientGrants {
		if g.GetClientID() == clientID && g.GetAudience() == audience {
			return g, nil
		}
	}
	return nil, nil
}
